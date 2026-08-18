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

// SetBaseMsg asks the root to retarget the pull request onto another branch.
//
// A name rather than a node id, because that is the only spelling
// updatePullRequest takes for this field.
type SetBaseMsg struct {
	ID   string
	Base string
}

// NeedBranchesMsg asks the root for a branch search this screen does not hold.
// The screen cannot fetch, so typing into the base picker reaches the root as a
// message and the request starts there.
//
// An empty Query is the search a picker opens over.
type NeedBranchesMsg struct {
	Repo  string
	Query string
}

// BranchSettleMsg is a wait that ran out on a filter that had stopped changing.
type BranchSettleMsg struct{ Query string }

// branchSettleDelay is how long the filter has to sit still before the search
// behind it is run. Typing a branch name a keystroke at a time would otherwise
// spend a request per letter, which is what commitSettleDelay exists to stop on
// the commit column.
const branchSettleDelay = 150 * time.Millisecond

// SetBranches hands the screen a branch search, and opens or refills the picker
// that asked for it.
//
// A picker already up gets its list replaced. That is a different move from
// opening one: the filter is the search, so rebuilding through NewPicker would
// clear the field that caused the fetch.
//
// A picker waiting to open takes the same terms SetRepo takes, and one more.
// The rail having focus is not the reader still standing on Base: enter starts
// the search and the ring is free the whole time it is out, so without the row
// itself a modal drops over whatever they walked to and takes the keyboard.
// Capturing covers the rest of that, including a merge form opened meanwhile,
// which a picker landing late would replace between one key and the next.
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

// branchNote is what the picker's title says about a search that matched more
// than it returned. Silence there reads as a repository with thirty branches.
func branchNote(more int) string {
	if more <= 0 {
		return ""
	}
	return strconv.Itoa(more) + " more · narrow the search"
}

// armBranches starts the wait for whatever is in the filter now.
func (m Model) armBranches() tea.Cmd {
	query := m.picking.p.Filter()
	return tea.Tick(branchSettleDelay, func(time.Time) tea.Msg {
		return BranchSettleMsg{Query: query}
	})
}

// settleBranches asks for the search behind a wait that ran out, and drops one
// the reader has typed past. Every keystroke arms its own, so a five-letter
// word sets five timers and only the last still names what is in the field.
//
// Nothing on the modal moves here. The list from before holds it until the root
// answers, which keeps a picker from blanking between one letter and the next
// on a search that comes back in a few milliseconds.
func (m Model) settleBranches(msg BranchSettleMsg) tea.Cmd {
	if m.picking.field != pickBase || m.picking.p.Filter() != msg.Query {
		return nil
	}
	// Already held, so there is nothing to ask for. The store refuses this too;
	// answering it here is what keeps the round trip off a filter being
	// backspaced through a search that has already landed.
	if m.branches.Loaded && m.branches.Query == msg.Query {
		return nil
	}

	repo, query := m.pr.Repository, msg.Query
	return func() tea.Msg { return NeedBranchesMsg{Repo: repo, Query: query} }
}

// baseItems is the branches as choices: the default branch first, then whatever
// the search returned, never the head branch, and always the branch already set.
//
// The default is pinned because it is where most retargets land, the way Copilot
// is pinned in the reviewer picker. It is pinned only while nothing has been
// typed: once there is a search the reader is looking for something specific,
// and a row at the top they did not ask for is a row enter can land on by
// accident.
//
// The head is dropped because GitHub refuses a pull request onto itself, and a
// row that can only fail is worse than no row. Never the branch already set,
// though, whatever it is called. A pull request from a fork carries the head's
// name and not its repository, so a contributor's main merging into this one's
// main matches here on the name alone; dropping it would leave the picker with
// nothing checked and enter would retarget onto whatever sorted first.
//
// Keeping the current base is what holds that line generally: the picker always
// opens on something checked, whatever the search returned.
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

// applyBase asks the root to retarget onto the branch the cursor was left on.
//
// The branch already set writes nothing. Opening the picker and pressing enter
// is how a reader backs out of one they opened by mistake, and the cursor starts
// on that row, so it is the likeliest thing to happen here.
func (m Model) applyBase(p picking) (Model, tea.Cmd) {
	chosen := p.p.Chosen()
	if len(chosen) != 1 || chosen[0] == m.railDetail().BaseRefName {
		return m, nil
	}

	id, base := m.pr.ID, chosen[0]
	return m, func() tea.Msg { return SetBaseMsg{ID: id, Base: base} }
}
