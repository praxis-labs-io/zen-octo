package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/comp"
	"github.com/zen-octo/zen-octo/internal/tui/prview"
)

// A retarget is applied here before it is sent, so both outcomes carry the key
// the store reconciles against. The failure carries the branch too, because the
// toast names what could not be done rather than the field it was done to.
type baseSetMsg struct {
	id  string
	key string
	res gh.BaseResult
}

type baseFailedMsg struct {
	id   string
	key  string
	base string
	err  error
}

// A branch search carries the query on both legs, so the store can tell an
// answer to the search being run from one to a search two keystrokes ago.
type branchesFetchedMsg struct {
	repo string
	res  gh.BranchResult
}

type branchesFailedMsg struct {
	repo  string
	query string
	err   error
}

// needBranches answers the screen asking for a branch search. The screen cannot
// fetch, so typing into the base picker reaches the root as a message and the
// request starts here.
func (m Model) needBranches(msg prview.NeedBranchesMsg) (tea.Model, tea.Cmd) {
	if !m.store.BeginBranches(msg.Repo, msg.Query) {
		return m, nil
	}
	return m, m.fetchBranches(msg.Repo, msg.Query)
}

func (m Model) fetchBranches(repo, query string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.Branches(ctx, repo, query)
		if err != nil {
			return branchesFailedMsg{repo: repo, query: query, err: err}
		}
		return branchesFetchedMsg{repo: repo, res: res}
	}
}

// branchesLanded hands the search to the screen, which opens the picker waiting
// on it or refills the one already up.
//
// Only to a screen showing that repository, for the reason repoMetaLanded gives:
// a response outlives the screen that asked for it, and one repository's
// branches offered against another are a list of writes GitHub refuses.
func (m Model) branchesLanded(msg branchesFetchedMsg) (tea.Model, tea.Cmd) {
	m.store.BranchesApplied(msg.repo, msg.res)

	if m.showingRepo(msg.repo) {
		m.detail.SetBranches(m.store.Branches(msg.repo))
	}
	return m, nil
}

// branchesFailed says so and leaves whatever the picker was showing. A modal
// blanked by a dropped connection reads as a repository with no branches.
func (m Model) branchesFailed(msg branchesFailedMsg) (tea.Model, tea.Cmd) {
	m.store.BranchesFailed(msg.repo, msg.query, msg.err)
	return m, m.toasts.Show(comp.ToastError, "Could not read the branches: "+msg.err.Error())
}

// setBase retargets a pull request, painting the new branch on the rail before
// the write leaves.
func (m Model) setBase(msg prview.SetBaseMsg) (tea.Model, tea.Cmd) {
	key := m.store.PendingBase(msg.ID, msg.Base)

	shown := m.detail.SetDetail(m.store.Detail(msg.ID))
	return m, tea.Batch(shown, m.sendBase(msg, key))
}

func (m Model) sendBase(msg prview.SetBaseMsg, key string) tea.Cmd {
	client := m.client

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.SetBase(ctx, msg.ID, msg.Base)
		if err != nil {
			return baseFailedMsg{id: msg.ID, key: key, base: msg.Base, err: err}
		}
		return baseSetMsg{id: msg.ID, key: key, res: res}
	}
}

// baseLanded takes GitHub's answer and then asks for everything the retarget
// invalidated, which is most of the screen.
//
// The detail, because a base change rewrites the behind-by count, the commit
// list, the changed-file count, the mergeability and the timeline, and the store
// can compute none of them.
//
// The diff, because it is every file that differs from a branch this pull
// request no longer targets. That one is the easiest to miss: the Files tab asks
// for a diff once per open, so nothing would ask again until the reader pressed
// the sync key while standing on that tab.
//
// The toast names GitHub's answer rather than the ask. They part company when
// somebody retargets in the browser first, and the rail is already showing what
// came back.
func (m Model) baseLanded(msg baseSetMsg) (tea.Model, tea.Cmd) {
	m.store.BaseApplied(msg.id, msg.key, msg.res)

	cmds := []tea.Cmd{m.toasts.Show(comp.ToastSuccess, "Now merging into "+msg.res.BaseRefName)}
	if m.showing(msg.id) {
		cmds = append(cmds, m.detail.SetDetail(m.store.Detail(msg.id)))
	}
	return m, tea.Batch(append(cmds, m.correctDetail(msg.id), m.correctFiles(msg.id))...)
}

// correctFiles asks for the diff again after a write that rewrote it, and is
// nil when there is none held.
//
// A reader who never opened the Files tab has nothing to correct: the first open
// fetches whatever is true by then. One who did is holding a diff against the
// old base, and the tab will not ask a second time on its own.
//
// It registers no refresh leg, for the reason correctDetail gives.
func (m Model) correctFiles(id string) tea.Cmd {
	if !m.store.Files(id).Loaded || !m.store.BeginFiles(id) {
		return nil
	}
	pr := m.store.Detail(id).Detail.PullRequest
	return m.fetchFiles(id, pr.Repository, pr.Number, pr.ChangedFiles)
}

// baseFailed is the revert branch. Nothing was typed, so the fetched branch and
// its count going back on the rail is the whole of it.
func (m Model) baseFailed(msg baseFailedMsg) (tea.Model, tea.Cmd) {
	m.store.EditReverted(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastError, "Could not merge into "+msg.base+": "+msg.err.Error())
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}
