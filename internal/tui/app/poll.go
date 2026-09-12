package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/config"
	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

// Carries its instant, so what is due is a comparison rather than a clock read inside Update.
type pollTickMsg struct{ at time.Time }

type checksTickMsg struct{ at time.Time }

type sectionPollFailedMsg struct {
	index int
	err   error
}

type pageFailedMsg struct {
	id  string
	err error
}

const (
	pollBeat = 5 * time.Second

	checksBeat = 10 * time.Second

	// Search indexing lags up to a minute, so asking the list faster returns the same rows.
	pollIdle = 30 * time.Second
)

type poller struct {
	sections []time.Time
	detailID string
	detailAt time.Time

	// Stays set while a tick is pending, even off-tab, so a quick return cannot arm a second chain.
	checksAt time.Time

	// Nothing else slows a failing page fetch: DetailFailed leaves its debt standing.
	pageID string
	pageAt time.Time
}

func (p *poller) stampSection(i, count int, at time.Time) {
	if i < 0 || i >= count {
		return
	}
	if len(p.sections) != count {
		p.sections = make([]time.Time, count)
	}
	p.sections[i] = at
}

func (p *poller) stampDetail(id string, at time.Time) {
	p.detailID, p.detailAt = id, at
}

func (p *poller) stampPageFailed(id string, at time.Time) {
	p.pageID, p.pageAt = id, at
}

func (p poller) pageDue(id string, at time.Time) bool {
	if id != p.pageID {
		return true
	}
	return at.Sub(p.pageAt) >= pollIdle
}

// A section that has never answered is not due: its first fetch is still out.
func (p poller) sectionDue(i int, at time.Time) bool {
	if i < 0 || i >= len(p.sections) || p.sections[i].IsZero() {
		return false
	}
	return at.Sub(p.sections[i]) >= pollIdle
}

func (p poller) detailDue(id string, every time.Duration, at time.Time) bool {
	if id != p.detailID {
		return true
	}
	return at.Sub(p.detailAt) >= every
}

func (p poller) checksDue(at time.Time) bool {
	return !p.checksAt.IsZero() && !at.Before(p.checksAt)
}

// Called only from Init and its own handler, so there is one chain rather than two at double rate.
func armPoll() tea.Cmd {
	return tea.Tick(pollBeat, func(at time.Time) tea.Msg { return pollTickMsg{at: at} })
}

func armChecks() tea.Cmd {
	return tea.Tick(checksBeat, func(at time.Time) tea.Msg { return checksTickMsg{at: at} })
}

func (m Model) startChecks() (Model, tea.Cmd) {
	if m.screen != screenDetail || !m.detail.ShowsChecks() || !m.poller.checksAt.IsZero() {
		return m, nil
	}
	m.poller.checksAt = time.Now().Add(checksBeat)
	return m, armChecks()
}

func (m Model) pollChecks(msg checksTickMsg) (tea.Model, tea.Cmd) {
	if m.screen != screenDetail || !m.detail.ShowsChecks() {
		m.poller.checksAt = time.Time{}
		return m, nil
	}
	if !m.poller.checksDue(msg.at) {
		return m, nil
	}

	m.poller.checksAt = msg.at.Add(checksBeat)
	next, job := armChecks(), m.detail.PollJob()
	id := m.detail.PullRequest().ID
	if id == "" || !m.poller.detailDue(id, checksBeat, msg.at) {
		return m, tea.Batch(next, job)
	}
	return m, tea.Batch(next, job, m.pulse(id))
}

func (m Model) poll(msg pollTickMsg) (tea.Model, tea.Cmd) {
	next := armPoll()

	if m.capturing() {
		return m, next
	}

	if m.screen == screenDetail {
		return m, tea.Batch(next, m.pollDetail(msg.at))
	}

	model, cmd := m.pollSectionDue(msg.at)
	return model, tea.Batch(next, cmd)
}

func (m Model) pollDetail(at time.Time) tea.Cmd {
	id := m.detail.PullRequest().ID
	if id == "" {
		return nil
	}

	owed := m.correctTimeline(id, at)
	if !m.poller.detailDue(id, m.detailEvery(id), at) {
		return owed
	}
	return tea.Batch(owed, m.pulse(id))
}

func (m Model) detailEvery(id string) time.Duration {
	if moving(m.store.Detail(id).Detail) {
		return pollBeat
	}
	return pollIdle
}

func moving(d gh.PullRequestDetail) bool {
	if d.State != gh.PRStateOpen {
		return false
	}
	switch d.Rollup.State {
	case gh.CheckStatePending, gh.CheckStateExpected:
		return true
	}
	return d.Merge == gh.MergeUnknown
}

func (m Model) correctTimeline(id string, at time.Time) tea.Cmd {
	if !m.store.StaleTimeline(id) || !m.detail.ShowsTimeline() {
		return nil
	}
	if !m.poller.pageDue(id, at) {
		return nil
	}

	pr := m.store.Detail(id).Detail.PullRequest
	if pr.ID == "" || !m.store.BeginDetail(id) {
		return nil
	}
	return m.fetchPage(id, pr.HeadRefName)
}

func (m Model) fetchPage(id, headRef string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.PullRequest(ctx, id, headRef)
		if err != nil {
			return pageFailedMsg{id: id, err: err}
		}
		return detailFetchedMsg{id: id, res: res}
	}
}

// Returns the model because store.Begin's counter is an int and does not survive a copy.
func (m Model) pollSectionDue(at time.Time) (Model, tea.Cmd) {
	i := m.list.ActiveIndex()
	if !m.poller.sectionDue(i, at) {
		return m, nil
	}

	sections := m.store.Sections()
	if i >= len(sections) || !sections[i].Loaded || !m.store.Begin(i) {
		return m, nil
	}
	return m, m.pollSection(i, sections[i].Filters)
}

func (m Model) pollSection(index int, query string) tea.Cmd {
	client, limit := m.client, m.limit

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.SearchPullRequests(ctx, config.ExpandQuery(query, time.Now()), limit)
		if err != nil {
			return sectionPollFailedMsg{index: index, err: err}
		}
		return sectionFetchedMsg{index: index, res: res}
	}
}
