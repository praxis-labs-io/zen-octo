package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/config"
	"github.com/praxis-labs-io/zen-octo/internal/gh"
)

// pollTickMsg is the heartbeat. It carries the instant it fired, so what is due
// is a comparison rather than a clock read inside Update.
type pollTickMsg struct{ at time.Time }

// checksTickMsg is the Checks tab's own heartbeat. It exists only while that
// tab is up; the general heartbeat remains one chain of its own.
type checksTickMsg struct{ at time.Time }

// sectionPollFailedMsg is a background section fetch that did not answer. The
// error rides along for the one reader waiting on it: a refresh that adopted it.
type sectionPollFailedMsg struct {
	index int
	err   error
}

// pageFailedMsg is a background page fetch that did not answer. It reaches the
// store and never the screen: nobody asked for this one, so nothing reports it.
type pageFailedMsg struct {
	id  string
	err error
}

const (
	// pollBeat is the tick, and how often a pull request still moving is re-asked.
	// CI is what a reader sits and watches.
	pollBeat = 5 * time.Second

	// checksBeat is how often the Checks tab re-asks even after CI has settled.
	checksBeat = 10 * time.Second

	// pollIdle is a pull request that has settled, and the list. Search indexing
	// lags up to a minute, so asking faster than this returns the same rows.
	pollIdle = 30 * time.Second
)

// poller is when each thing last answered, plus the live Checks chain. A screen
// coming back into view refreshes what has aged instead of waiting out a fresh
// interval.
type poller struct {
	sections []time.Time
	detailID string
	detailAt time.Time

	// checksAt is when the live Checks chain is next due. It stays set while one
	// tick is pending, including off-tab, so a quick return cannot arm a second.
	checksAt time.Time

	// pageID and pageAt are when a background page fetch last failed. Nothing
	// else slows one down: DetailFailed leaves the debt it was for standing.
	pageID string
	pageAt time.Time
}

// stampSection records that a section answered, whether or not it answered well.
// A failure costs one interval rather than a retry every beat.
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

// pageDue is whether a background page fetch is worth making. One that has never
// failed goes at once; one that has costs an interval, since it is megabytes.
func (p poller) pageDue(id string, at time.Time) bool {
	if id != p.pageID {
		return true
	}
	return at.Sub(p.pageAt) >= pollIdle
}

// sectionDue is whether that section has gone long enough without an answer. One
// that has never answered is not due: its first fetch is still out.
func (p poller) sectionDue(i int, at time.Time) bool {
	if i < 0 || i >= len(p.sections) || p.sections[i].IsZero() {
		return false
	}
	return at.Sub(p.sections[i]) >= pollIdle
}

// detailDue is the same for the pull request on screen. One never stamped reads
// as due, and BeginPulse is what refuses it while nothing is loaded.
func (p poller) detailDue(id string, every time.Duration, at time.Time) bool {
	if id != p.detailID {
		return true
	}
	return at.Sub(p.detailAt) >= every
}

// checksDue rejects a second chain firing ahead of the live one's next beat.
// Zero means no tick is pending, so a message left behind by an old chain dies.
func (p poller) checksDue(at time.Time) bool {
	return !p.checksAt.IsZero() && !at.Before(p.checksAt)
}

// armPoll schedules the next beat. Called from Init and from the handler below
// and nowhere else: one chain with one start is what stops two at double rate.
func armPoll() tea.Cmd {
	return tea.Tick(pollBeat, func(at time.Time) tea.Msg { return pollTickMsg{at: at} })
}

func armChecks() tea.Cmd {
	return tea.Tick(checksBeat, func(at time.Time) tea.Msg { return checksTickMsg{at: at} })
}

// startChecks starts the tab's chain once. checksAt names the tick it expects,
// so a command left behind by an earlier visit cannot become a second chain.
func (m Model) startChecks() (Model, tea.Cmd) {
	if m.screen != screenDetail || !m.detail.ShowsChecks() || !m.poller.checksAt.IsZero() {
		return m, nil
	}
	m.poller.checksAt = time.Now().Add(checksBeat)
	return m, armChecks()
}

// pollChecks re-asks the volatile detail fields while Checks is in front of the
// reader. Leaving the tab ends the chain when its one outstanding tick lands.
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

// poll asks for whatever the screen in front of the reader has let go stale.
// Nothing here takes a refresh leg, so a beat that fetches still says nothing.
func (m Model) poll(msg pollTickMsg) (tea.Model, tea.Cmd) {
	next := armPoll()

	// A picker or a form has the keyboard, and an answer landing under one
	// relayouts the page it is drawn over.
	if m.detail.Capturing() {
		return m, next
	}

	if m.screen == screenDetail {
		return m, tea.Batch(next, m.pollDetail(msg.at))
	}

	// The model comes back rather than the command alone: store.Begin stamps the
	// section against a counter that is an int, and an int does not survive a copy.
	model, cmd := m.pollSectionDue(msg.at)
	return model, tea.Batch(next, cmd)
}

// pollDetail re-asks the pull request on screen, and asks for the whole page
// where a pulse has already reported something it cannot carry.
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

// detailEvery is how often this pull request is worth re-asking.
func (m Model) detailEvery(id string) time.Duration {
	if moving(m.store.Detail(id).Detail) {
		return pollBeat
	}
	return pollIdle
}

// moving is a pull request with something of its own in flight: checks running,
// or a mergeability GitHub has not finished working out.
func moving(d gh.PullRequestDetail) bool {
	// Nothing about a merged or closed one is still settling, and a check left
	// pending on one would otherwise hold it at the fast interval for good.
	if d.State != gh.PRStateOpen {
		return false
	}
	switch d.Rollup.State {
	case gh.CheckStatePending, gh.CheckStateExpected:
		return true
	}
	return d.Merge == gh.MergeUnknown
}

// correctTimeline asks for the whole page once a pulse reports something it
// cannot carry, gated on the conversation: the only tab any of it reaches.
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

// fetchPage is fetchDetail with the one branch that differs, the way pollSection
// is fetchSection with one: a page that arrives is the same page either way.
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

// pollSectionDue re-asks the section the tab strip is on. Only that one: the
// others are off screen, and their counts follow when the reader arrives.
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

// pollSection is fetchSection with the one branch that differs: a failure nobody
// asked for reports itself to the store and not to the screen.
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
