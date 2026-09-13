package prview

import (
	"image/color"
	"slices"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
)

type SetBaseMsg struct {
	ID   string
	Base string
}

type NeedBranchesMsg struct {
	Repo  string
	Query string
}

type BranchSettleMsg struct{ Query string }

const branchSettleDelay = 150 * time.Millisecond

// SetBranches holds a branch search, refilling an open base picker or opening one still waiting on it.
func (m *Model) SetBranches(b store.Branches) {
	m.branches = b
	if !b.Loaded {
		return
	}

	if m.picking.field == pickBase {
		m.picking.p.Replace(
			baseItems(b, m.pr, m.railDetail().BaseRefName, m.theme.Text),
			branchNote(b.More),
		)
		return
	}

	if m.picking.want != pickBase {
		return
	}
	m.picking.want = pickNone

	if m.Capturing() || !m.railVisible() || m.focus != paneRail {
		return
	}
	if m.railRing.on.kind != focusBase {
		return
	}
	m.startPicker(pickBase)
}

func branchNote(more int) string {
	if more <= 0 {
		return ""
	}
	return strconv.Itoa(more) + " more · narrow the search"
}

func (m Model) armBranches() tea.Cmd {
	query := m.picking.p.Filter()
	return tea.Tick(branchSettleDelay, func(time.Time) tea.Msg {
		return BranchSettleMsg{Query: query}
	})
}

func (m Model) settleBranches(msg BranchSettleMsg) tea.Cmd {
	if m.picking.field != pickBase || m.picking.p.Filter() != msg.Query {
		return nil
	}
	if m.branches.Loaded && m.branches.Query == msg.Query {
		return nil
	}

	repo, query := m.pr.Repository, msg.Query
	return func() tea.Msg { return NeedBranchesMsg{Repo: repo, Query: query} }
}

// The current base is always kept: a fork's head can share its name, and a picker opened with nothing checked retargets on enter.
func baseItems(b store.Branches, pr gh.PullRequest, base string, c color.Color) []comp.PickerItem {
	names := make([]string, 0, len(b.Names)+2)
	if b.Query == "" && b.Default != "" {
		names = append(names, b.Default)
	}
	names = append(names, b.Names...)
	if base != "" {
		names = append(names, base)
	}

	out := make([]comp.PickerItem, 0, len(names))
	for _, n := range names {
		if n != base && n == pr.HeadRefName {
			continue
		}
		if slices.ContainsFunc(out, func(it comp.PickerItem) bool { return it.ID == n }) {
			continue
		}
		out = append(out, comp.PickerItem{ID: n, Name: n, Color: c})
	}
	return out
}

func (m Model) applyBase(p picking) (Model, tea.Cmd) {
	chosen := p.p.Chosen()
	if len(chosen) != 1 || chosen[0] == m.railDetail().BaseRefName {
		return m, nil
	}

	id, base := m.pr.ID, chosen[0]
	return m, func() tea.Msg { return SetBaseMsg{ID: id, Base: base} }
}
