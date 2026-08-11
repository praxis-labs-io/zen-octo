package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/prview"
)

// The choices a picker offers are the repository's, so they are named by it
// rather than by the pull request that asked. Two screens on one repository
// share the answer, which is why the store refuses the second request.
type repoMetaFetchedMsg struct {
	repo string
	res  gh.RepoMetaResult
}

type repoMetaFailedMsg struct {
	repo string
	err  error
}

// A label set is applied here before it is sent, so both outcomes name the
// write rather than the pull request alone. The failure carries nothing back:
// nothing was typed, and the store puts the fetched set back on its own.
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

// needRepoMeta answers a screen that wants a picker it has no choices for. The
// store refuses a repository already in flight or already held, so a reader
// opening the same picker twice costs one request.
func (m Model) needRepoMeta(repo string) (tea.Model, tea.Cmd) {
	if !m.store.BeginRepoMeta(repo) {
		// Already held, and the screen asked because it had not been handed
		// them yet. Nothing to fetch, so hand them over now.
		if held := m.store.Repo(repo); held.Loaded && m.showingRepo(repo) {
			m.detail.SetRepo(held)
		}
		return m, nil
	}
	return m, m.fetchRepoMeta(repo)
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

// repoMetaLanded stores the choices and hands them to the screen, which opens
// the picker that was waiting on them.
//
// Only to a screen showing that repository. A response outlives the screen that
// asked for it, and handing one repository's labels to a pull request in
// another opens a picker where nothing reads as checked and whose ids GitHub
// rejects on apply. This is the repository-level twin of showing(id).
func (m Model) repoMetaLanded(msg repoMetaFetchedMsg) (tea.Model, tea.Cmd) {
	m.store.RepoMetaApplied(msg.repo, msg.res)

	if m.showingRepo(msg.repo) {
		m.detail.SetRepo(m.store.Repo(msg.repo))
	}
	return m, nil
}

// showingRepo is whether the detail screen is up on a pull request in this
// repository.
func (m Model) showingRepo(repo string) bool {
	return m.screen == screenDetail && m.detail.PullRequest().Repository == repo
}

// repoMetaFailed says so and leaves the picker unopened. A modal over an empty
// list reads as a repository with no labels, which is a worse lie than no
// modal at all.
func (m Model) repoMetaFailed(msg repoMetaFailedMsg) (tea.Model, tea.Cmd) {
	m.store.RepoMetaFailed(msg.repo, msg.err)
	return m, m.toasts.Show(comp.ToastError, "Could not read the repository: "+msg.err.Error())
}

// setLabels writes a label set, painting it on the rail before the write
// leaves. The rail changing is the acknowledgement, the way the optimistic
// comment is one for a comment.
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

// labelsLanded takes GitHub's answer, which is the authority on what the pull
// request now carries: a label deleted from the repository since the picker was
// filled comes back absent whatever was asked for.
func (m Model) labelsLanded(msg labelsSetMsg) (tea.Model, tea.Cmd) {
	m.store.LabelsApplied(msg.id, msg.key, msg.res)

	toast := m.toasts.Show(comp.ToastSuccess, "Labels updated")
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}

// labelsFailed is the revert branch. Nothing was typed, so the fetched set
// going back on the rail is the whole of it.
func (m Model) labelsFailed(msg labelsFailedMsg) (tea.Model, tea.Cmd) {
	m.store.EditReverted(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastError, "Could not set the labels: "+msg.err.Error())
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}
