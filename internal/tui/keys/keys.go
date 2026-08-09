// Package keys declares every binding once, with its help text attached, so the
// help view renders from the same declaration Update matches on. A rebind moves
// both or neither.
package keys

import "charm.land/bubbles/v2/key"

// GlobalMap answers whatever has focus. The root model handles these before it
// delegates to a screen.
//
// Quit and ForceQuit are separate because the compose pane takes plain letters
// as text. While it is open the root stands aside and q types a q; ctrl+c still
// quits, because one way out has to work from everywhere.
type GlobalMap struct {
	Help      key.Binding
	Quit      key.Binding
	ForceQuit key.Binding
}

// ListMap is live while the pull request list has focus.
type ListMap struct {
	Up           key.Binding
	Down         key.Binding
	Top          key.Binding
	Bottom       key.Binding
	PageUp       key.Binding
	PageDown     key.Binding
	HalfPageUp   key.Binding
	HalfPageDown key.Binding
	NextSection  key.Binding
	PrevSection  key.Binding
	Open         key.Binding
	Sync         key.Binding
}

// DetailMap is live on the pull request detail screen. The same movement keys
// serve every pane; focus decides what they move.
//
// FocusPane is one binding over the digits rather than one per pane, because
// the Files tab puts a third pane on screen and the panes are numbered by where
// they sit rather than by what they hold.
//
// FocusNext and FocusPrev own tab and shift+tab, which the tab strip used to
// answer to as well. The strip keeps the brackets: the ring is the key a reader
// reaches for many times on one pull request, and the strip a handful.
type DetailMap struct {
	Up           key.Binding
	Down         key.Binding
	Top          key.Binding
	Bottom       key.Binding
	PageUp       key.Binding
	PageDown     key.Binding
	HalfPageUp   key.Binding
	HalfPageDown key.Binding
	NextTab      key.Binding
	PrevTab      key.Binding
	NextFile     key.Binding
	PrevFile     key.Binding
	FocusNext    key.Binding
	FocusPrev    key.Binding
	PaneLeft     key.Binding
	PaneRight    key.Binding
	FocusPane    key.Binding
	ToggleRail   key.Binding
	Expand       key.Binding
	Sync         key.Binding
	Back         key.Binding

	// The compose pane's own. Comment opens it; Post and Editor are live only
	// while it is open. Closing it is Back and reaching the post button is
	// FocusNext, because both already mean that everywhere else on this screen.
	//
	// Post is a chord only a terminal speaking the Kitty keyboard protocol can
	// send. Every terminal reaches the button instead, which is why there is
	// one, and the pane names whichever of the two the reader can actually use.
	Comment  key.Binding
	Post     key.Binding
	Activate key.Binding
	Editor   key.Binding

	// Reply and QuoteReply answer a review thread, from the comment the ring is
	// on. They take r and R, which is why syncing moved to s: replying is the
	// key this screen exists for, and it was behind the one that refetches.
	Reply      key.Binding
	QuoteReply key.Binding

	// NextWithin and PrevWithin move between the comments inside the focused
	// thread. Tab walks whole threads, because a thread is one thing to read and
	// stopping on every reply in a long one makes crossing the page a chore.
	// This is the level below it, for the times the answer is to a reply rather
	// than to the thread.
	NextWithin key.Binding
	PrevWithin key.Binding
}

// Global, List, and Detail are the declarations. Config-driven rebinding lands
// later; until then these are the only place a key is named.
var (
	Global = GlobalMap{
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:      key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		ForceQuit: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit from anywhere")),
	}

	List = ListMap{
		Up:           key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:         key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		Top:          key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:       key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		PageUp:       key.NewBinding(key.WithKeys("pgup", "ctrl+b"), key.WithHelp("pgup", "page up")),
		PageDown:     key.NewBinding(key.WithKeys("pgdown", "ctrl+f"), key.WithHelp("pgdn", "page down")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "half page up")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "half page down")),
		NextSection:  key.NewBinding(key.WithKeys("]", "tab"), key.WithHelp("]/tab", "next section")),
		PrevSection:  key.NewBinding(key.WithKeys("[", "shift+tab"), key.WithHelp("[", "prev section")),
		Open:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Sync:         key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sync")),
	}

	Detail = DetailMap{
		Up:           key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:         key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		Top:          key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:       key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		PageUp:       key.NewBinding(key.WithKeys("pgup", "ctrl+b"), key.WithHelp("pgup", "page up")),
		PageDown:     key.NewBinding(key.WithKeys("pgdown", "ctrl+f"), key.WithHelp("pgdn", "page down")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "half page up")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "half page down")),
		NextTab:      key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next tab")),
		PrevTab:      key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev tab")),
		NextFile:     key.NewBinding(key.WithKeys("}"), key.WithHelp("}", "next file")),
		PrevFile:     key.NewBinding(key.WithKeys("{"), key.WithHelp("{", "prev file")),
		FocusNext:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "focus next")),
		FocusPrev:    key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "focus prev")),
		PaneLeft:     key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h/←", "pane left")),
		PaneRight:    key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/→", "pane right")),
		FocusPane:    key.NewBinding(key.WithKeys("1", "2", "3"), key.WithHelp("1/2/3", "focus pane")),
		ToggleRail:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "toggle details")),
		Expand:       key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "expand or collapse")),
		Sync:         key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sync")),
		Back:         key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),

		Comment:    key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "comment")),
		Post:       key.NewBinding(key.WithKeys("ctrl+enter"), key.WithHelp("ctrl+enter", "post")),
		Activate:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "press the button")),
		Editor:     key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "$EDITOR")),
		Reply:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reply")),
		QuoteReply: key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "quote reply")),
		NextWithin: key.NewBinding(key.WithKeys("J"), key.WithHelp("J", "next in thread")),
		PrevWithin: key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "prev in thread")),
	}
)

// ShortHelp is the one line the status bar carries.
func (k ListMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Down, k.Open, k.NextSection, k.Sync, Global.Help, Global.Quit}
}

// FullHelp is the overlay. Every binding in the map appears here; a test holds
// that, so adding a binding without a home in the help fails the build.
func (k ListMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown},
		{k.NextSection, k.PrevSection, k.Open, k.Sync},
		{Global.Help, Global.Quit, Global.ForceQuit},
	}
}

// ShortHelp is the one line the status bar carries. Sync is in the overlay
// only: a seventh hint pushes the line past the pull request number on the
// right at 100 columns, and the number is what says which one is on screen.
//
// FocusNext is in the overlay for a different reason. The bar is the same line
// on every tab, and the ring is only on the one without a column; hinting it
// beside a pane where it does nothing is worse than not hinting it at all.
func (k DetailMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Down, k.NextTab, k.Expand, k.ToggleRail, k.Back, Global.Help}
}

// FullHelp is the overlay.
func (k DetailMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown},
		{k.NextTab, k.PrevTab, k.NextFile, k.PrevFile},
		{k.FocusNext, k.FocusPrev, k.Expand, k.ToggleRail},
		{k.PaneLeft, k.PaneRight, k.FocusPane},
		{k.NextWithin, k.PrevWithin, k.Reply, k.QuoteReply},
		{k.Comment, k.Post, k.Activate, k.Editor},
		{k.Sync, k.Back, Global.Help, Global.Quit, Global.ForceQuit},
	}
}
