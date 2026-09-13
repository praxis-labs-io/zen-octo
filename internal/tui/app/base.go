package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

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

type branchesFetchedMsg struct {
	repo string
	res  gh.BranchResult
}

type branchesFailedMsg struct {
	repo  string
	query string
	err   error
}

// A search already held is handed over, since each new screen asks again and BeginBranches refuses it.
func (m Model) needBranches(msg prview.NeedBranchesMsg) (tea.Model, tea.Cmd) {
	if m.store.BeginBranches(msg.Repo, msg.Query) {
		return m, m.fetchBranches(msg.Repo, msg.Query)
	}

	held := m.store.Branches(msg.Repo)
	if held.Loaded && held.Query == msg.Query && m.showingRepo(msg.Repo) {
		m.detail.SetBranches(held)
	}
	return m, nil
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

func (m Model) branchesLanded(msg branchesFetchedMsg) (tea.Model, tea.Cmd) {
	m.store.BranchesApplied(msg.repo, msg.res)

	held := m.store.Branches(msg.repo)
	if held.Query == msg.res.Query && m.showingRepo(msg.repo) {
		m.detail.SetBranches(held)
	}
	return m, nil
}

func (m Model) branchesFailed(msg branchesFailedMsg) (tea.Model, tea.Cmd) {
	if m.store.Branches(msg.repo).Query != msg.query {
		return m, nil
	}

	m.store.BranchesFailed(msg.repo, msg.query, msg.err)
	return m, m.toasts.Show(comp.ToastError, "Could not read the branches: "+msg.err.Error())
}

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

// Only the detail is refetched; the diff waits for the changed-file count that arrives with it.
func (m Model) baseLanded(msg baseSetMsg) (tea.Model, tea.Cmd) {
	m.store.BaseApplied(msg.id, msg.key, msg.res)

	cmds := []tea.Cmd{m.toasts.Show(comp.ToastSuccess, "Now merging into "+msg.res.BaseRefName)}
	if m.showing(msg.id) {
		cmds = append(cmds, m.detail.SetDetail(m.store.Detail(msg.id)))
	}
	return m, tea.Batch(append(cmds, m.correctDetail(msg.id))...)
}

func (m Model) correctFiles(id string) tea.Cmd {
	if !m.store.StaleFiles(id) || !m.store.Files(id).Loaded {
		return nil
	}
	pr := m.store.Detail(id).Detail.PullRequest
	if pr.ID == "" || !m.store.BeginFiles(id) {
		return nil
	}
	return m.fetchFiles(id, pr.Repository, pr.Number, pr.ChangedFiles)
}

func (m Model) baseFailed(msg baseFailedMsg) (tea.Model, tea.Cmd) {
	m.store.EditReverted(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastError, "Could not merge into "+msg.base+": "+msg.err.Error())
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}
