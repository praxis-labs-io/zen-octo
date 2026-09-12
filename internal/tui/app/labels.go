package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

type repoMetaFetchedMsg struct {
	repo string
	res  gh.RepoMetaResult
}

type repoMetaFailedMsg struct {
	repo string
	err  error
}

type labelsSetMsg struct {
	id  string
	key string
	res gh.LabelsResult
}

type labelsFailedMsg struct {
	id  string
	key string
	err error
}

func (m Model) needRepoMeta(repo string) (tea.Model, tea.Cmd) {
	if !m.store.BeginRepoMeta(repo) {
		if held := m.store.Repo(repo); held.Loaded && m.showingRepo(repo) {
			return m, m.detail.SetRepo(held)
		}
		return m, nil
	}

	var shown []tea.Cmd
	if m.showingRepo(repo) {
		shown = append(shown, m.detail.SetRepo(m.store.Repo(repo)), m.detail.Init())
	}
	return m, tea.Batch(append(shown, m.fetchRepoMeta(repo))...)
}

func (m Model) fetchRepoMeta(repo string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.RepoMeta(ctx, repo)
		if err != nil {
			return repoMetaFailedMsg{repo: repo, err: err}
		}
		return repoMetaFetchedMsg{repo: repo, res: res}
	}
}

func (m Model) repoMetaLanded(msg repoMetaFetchedMsg) (tea.Model, tea.Cmd) {
	m.store.RepoMetaApplied(msg.repo, msg.res)

	if m.showingRepo(msg.repo) {
		return m, m.detail.SetRepo(m.store.Repo(msg.repo))
	}
	return m, nil
}

func (m Model) showingRepo(repo string) bool {
	return m.screen == screenDetail && m.detail.PullRequest().Repository == repo
}

// Hands the failed record to the screen anyway, so an open mention popup can say the list is not coming.
func (m Model) repoMetaFailed(msg repoMetaFailedMsg) (tea.Model, tea.Cmd) {
	m.store.RepoMetaFailed(msg.repo, msg.err)

	toast := m.toasts.Show(comp.ToastError, "Could not read the repository: "+msg.err.Error())
	if !m.showingRepo(msg.repo) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetRepo(m.store.Repo(msg.repo)), toast)
}

func (m Model) setLabels(msg prview.SetLabelsMsg) (tea.Model, tea.Cmd) {
	key := m.store.PendingLabels(msg.ID, msg.Labels)

	shown := m.detail.SetDetail(m.store.Detail(msg.ID))
	return m, tea.Batch(shown, m.sendLabels(msg, key))
}

func (m Model) sendLabels(msg prview.SetLabelsMsg, key string) tea.Cmd {
	client := m.client

	ids := make([]string, 0, len(msg.Labels))
	for _, l := range msg.Labels {
		ids = append(ids, l.ID)
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.SetLabels(ctx, msg.ID, ids)
		if err != nil {
			return labelsFailedMsg{id: msg.ID, key: key, err: err}
		}
		return labelsSetMsg{id: msg.ID, key: key, res: res}
	}
}

func (m Model) labelsLanded(msg labelsSetMsg) (tea.Model, tea.Cmd) {
	m.store.LabelsApplied(msg.id, msg.key, msg.res)

	toast := m.toasts.Show(comp.ToastSuccess, "Labels updated")
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}

func (m Model) labelsFailed(msg labelsFailedMsg) (tea.Model, tea.Cmd) {
	m.store.EditReverted(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastError, "Could not set the labels: "+msg.err.Error())
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}
