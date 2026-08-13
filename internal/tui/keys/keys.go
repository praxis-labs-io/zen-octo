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
// NextBlock and PrevBlock take the braces, which are paragraph motion in vim
// and mean the same thing here: go to the next block on this screen. What a
// block is belongs to the tab. On the conversation it is a card or a thread and
// the key walks the focus ring; on the two tabs showing a diff it is a file.
// The two used to be separate keys for what a reader does with one intention,
// and one of them then sat inert on the tab where the other worked.
//
// That gives tab and shift+tab back to the tab strip, which is what they do on
// the list screen. A reader crossing the two screens presses the same key for
// the same move.
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
	NextBlock    key.Binding
	PrevBlock    key.Binding
	PaneLeft     key.Binding
	PaneRight    key.Binding
	FocusPane    key.Binding
	ToggleRail   key.Binding
	Expand       key.Binding
	Sync         key.Binding
	Back         key.Binding

	// The compose pane's own. Comment opens it; Post and Editor are live only
	// while it is open. Closing it is Back, and the button is reached with
	// Form.Next rather than anything here.
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

	// Resolve settles a review thread and opens a settled one. One key for both,
	// because GitHub treats it as one control with two permissions and the card
	// names whichever of the two the reader has.
	//
	// Jump takes the thread to its place in the diff, which is the one question
	// a comment on a line raises that the conversation cannot answer.
	Resolve key.Binding
	Jump    key.Binding

	// Toggle checks and unchecks a row in a picker that takes a set. It is live
	// only while one is open, which is why it can take space: nothing else on
	// this screen wants it, and a picker owns the keyboard whenever it is up.
	Toggle key.Binding
}

// FormMap walks the fields of a compose box or the merge form, and is live only
// while one of them holds the keyboard.
//
// It is its own map because tab means something else on the screen behind it,
// and the two can never be live at once: a box takes every key until it is
// closed. The braces the screen walks blocks with are text in a textarea, so
// tab is the only key left that can move a caret out of one.
type FormMap struct {
	Next key.Binding
	Prev key.Binding
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
		NextSection:  key.NewBinding(key.WithKeys("]", "tab"), key.WithHelp("]/tab", "next tab")),
		PrevSection:  key.NewBinding(key.WithKeys("[", "shift+tab"), key.WithHelp("[", "prev tab")),
		Open:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open")),
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
		NextTab:      key.NewBinding(key.WithKeys("]", "tab"), key.WithHelp("]/tab", "next tab")),
		PrevTab:      key.NewBinding(key.WithKeys("[", "shift+tab"), key.WithHelp("[", "prev tab")),
		NextBlock:    key.NewBinding(key.WithKeys("}"), key.WithHelp("}", "next block")),
		PrevBlock:    key.NewBinding(key.WithKeys("{"), key.WithHelp("{", "prev block")),
		PaneLeft:     key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h/←", "pane left")),
		PaneRight:    key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/→", "pane right")),
		FocusPane:    key.NewBinding(key.WithKeys("1", "2", "3"), key.WithHelp("1/2/3", "focus pane")),
		ToggleRail:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "details")),
		Expand:       key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "expand")),
		Sync:         key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sync")),
		Back:         key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),

		Comment:    key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "comment")),
		Post:       key.NewBinding(key.WithKeys("ctrl+enter"), key.WithHelp("ctrl+⏎", "post")),
		Activate:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open or press")),
		Editor:     key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "$EDITOR")),
		Reply:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reply")),
		QuoteReply: key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "quote reply")),
		NextWithin: key.NewBinding(key.WithKeys("J"), key.WithHelp("J", "next in thread")),
		PrevWithin: key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "prev in thread")),
		Resolve:    key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "resolve or unresolve")),
		Jump:       key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "show in the diff")),
		Toggle:     key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "check or uncheck")),
	}

	Form = FormMap{
		Next: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
		Prev: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev field")),
	}
)

// hint restates a binding for the status bar, where a pair of opposed keys
// shares one verb: the bar has room for "j/k move" and not for a line naming
// up and down separately. It keeps the real key list, so the help stays
// answerable to the declarations.
//
// The overlay is the other way round. It has a row per binding and no reader
// looking for the one word that gets them moving, so "up" and "down" belong
// there and a shared verb would leave two rows reading the same.
func hint(b key.Binding, keys, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(b.Keys()...), key.WithHelp(keys, desc))
}

// ShortHelp is the one line the status bar carries.
func (k ListMap) ShortHelp() []key.Binding {
	return []key.Binding{
		hint(k.Down, "j/k", "move"),
		k.Open,
		hint(k.NextSection, "[/]", "tab"),
		k.Sync,
		Global.Help,
		Global.Quit,
	}
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
// only: an eighth hint pushes the line past the pull request number on the
// right at 100 columns, and the number is what says which one is on screen.
//
// The block keys are on it now that one pair means the same move on every tab.
// While they were two, either one named a key that did nothing on the tab the
// reader was looking at.
func (k DetailMap) ShortHelp() []key.Binding {
	return []key.Binding{
		hint(k.Down, "j/k", "move"),
		hint(k.NextBlock, "{/}", "block"),
		k.Expand,
		k.ToggleRail,
		hint(k.NextTab, "[/]", "tab"),
		k.Back,
		Global.Help,
	}
}

// FullHelp is the overlay. The form keys are on it as well: they are live only
// while a box has the keyboard, but the reader asking what tab does is owed
// both answers.
func (k DetailMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown},
		{k.NextTab, k.PrevTab, k.NextBlock, k.PrevBlock},
		{k.PaneLeft, k.PaneRight, k.FocusPane},
		{k.Expand, k.ToggleRail},
		{k.NextWithin, k.PrevWithin, k.Reply, k.QuoteReply},
		{k.Resolve, k.Jump},
		{k.Comment, k.Post, k.Activate, k.Editor},
		{Form.Next, Form.Prev, k.Toggle},
		{k.Sync, k.Back, Global.Help, Global.Quit, Global.ForceQuit},
	}
}
