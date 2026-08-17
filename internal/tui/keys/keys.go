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

	// CopyLink and Browse take the pull request under the cursor out of the
	// terminal. Both mean the pull request itself, on either screen.
	CopyLink key.Binding
	Browse   key.Binding
}

// DetailMap is live on the pull request detail screen. The same movement keys
// serve every pane; focus decides what they move.
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

	// NextFile and PrevFile are the coarser move, on the tab that shows one file
	// at a time. The strip keeps ] and [, which is what the list screen uses.
	NextFile key.Binding
	PrevFile key.Binding

	// The braces are paragraph motion in vim and mean the same here: go to the
	// next block. What a block is belongs to the tab.
	NextBlock key.Binding
	PrevBlock key.Binding

	PaneLeft  key.Binding
	PaneRight key.Binding

	// FocusPane is one binding over the digits rather than one per pane: the
	// Files tab puts a third on screen, and they are numbered by where they sit.
	FocusPane key.Binding

	ToggleRail key.Binding
	Expand     key.Binding
	Sync       key.Binding
	Back       key.Binding

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

	// React opens the list of GitHub's eight over the block the ring is on and
	// toggles the one chosen. It takes + because that is the button GitHub puts
	// on every comment, and because it is the one thing a reader does to
	// somebody else's writing without writing anything back.
	React key.Binding

	// Edit opens the box over the block the ring is on, with its words in it.
	// Delete takes one off, behind a confirm. It is shifted because deleting is
	// not something to do by leaning on a key, and the confirm is there because
	// shift guards against the wrong key rather than against the wrong card.
	Edit   key.Binding
	Delete key.Binding

	// Resolve settles a review thread and opens a settled one. One key for both,
	// because GitHub treats it as one control with two permissions and the card
	// names whichever of the two the reader has.
	//
	// Jump takes the thread to its place in the diff, which is the one question
	// a comment on a line raises that the conversation cannot answer.
	Resolve key.Binding
	Jump    key.Binding

	// CopyLink and Browse mean the pull request on all four tabs and never the
	// block the ring is on: nothing but the pull request carries a URL.
	CopyLink key.Binding
	Browse   key.Binding
}

// FormMap is live while a compose box, the merge form or a picker holds the
// keyboard, which is never while the keys behind it mean anything else.
type FormMap struct {
	// tab is the only key that can move a caret out of a textarea: the braces
	// the screen walks blocks with are text once a box is open.
	Next key.Binding
	Prev key.Binding

	// Toggle checks and unchecks the row the cursor is on, in a picker that
	// takes a set and on the merge form's delete row. Both are that one control.
	Toggle key.Binding
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
		CopyLink:     key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy link")),
		Browse:       key.NewBinding(key.WithKeys("O"), key.WithHelp("O", "open in browser")),
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
		NextFile:     key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next file")),
		PrevFile:     key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev file")),
		NextBlock:    key.NewBinding(key.WithKeys("}"), key.WithHelp("}", "next block")),
		PrevBlock:    key.NewBinding(key.WithKeys("{"), key.WithHelp("{", "prev block")),
		PaneLeft:     key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h/←", "pane left")),
		PaneRight:    key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/→", "pane right")),
		FocusPane:    key.NewBinding(key.WithKeys("1", "2", "3"), key.WithHelp("1/2/3", "focus pane")),
		ToggleRail:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "details")),
		Expand:       key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "expand")),
		Sync:         key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sync")),
		Back:         key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),

		Comment:    key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "comment")),
		Post:       key.NewBinding(key.WithKeys("ctrl+enter"), key.WithHelp("ctrl+⏎", "post")),
		Activate:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open or press")),
		Editor:     key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "$EDITOR")),
		Reply:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reply")),
		QuoteReply: key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "quote reply")),
		React:      key.NewBinding(key.WithKeys("+"), key.WithHelp("+", "react")),
		Edit:       key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		Delete:     key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "delete")),
		Resolve:    key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "resolve or unresolve")),
		Jump:       key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "show in the diff")),
		CopyLink:   key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy link")),
		Browse:     key.NewBinding(key.WithKeys("O"), key.WithHelp("O", "open in browser")),
	}

	Form = FormMap{
		Next:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
		Prev:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev field")),
		Toggle: key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "check or uncheck")),
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
		k.CopyLink,
		hint(k.Browse, "O", "browser"),
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
		{k.CopyLink, k.Browse},
		{Global.Help, Global.Quit, Global.ForceQuit},
	}
}

// DetailContext is what the detail screen can do where the reader is standing.
// The bar is one line on four tabs that hold different things, and a hint for a
// key that is inert on the tab under it is worse than no hint at all: the
// reader presses it, nothing happens, and the whole line stops being worth
// reading.
type DetailContext struct {
	// Blocks is whether the braces have anything to walk. Every tab but Checks
	// does: cards on the conversation, files in a diff.
	Blocks bool

	// Expand is whether there is something to open. The conversation has folds
	// and the Files tab has directories; the other two have neither.
	Expand bool

	// Rail is whether there is a rail to toggle, which the tabs with a column
	// have no room for.
	Rail bool

	// Files is whether there is another file to go to, which is the one tab
	// showing one at a time.
	Files bool
}

// ShortHelp is the one line the status bar carries. Sync is in the overlay
// only, to keep the line inside a hundred columns.
func (k DetailMap) ShortHelp(c DetailContext) []key.Binding {
	out := []key.Binding{hint(k.Down, "j/k", "move")}
	if c.Blocks {
		out = append(out, hint(k.NextBlock, "{/}", "block"))
	}
	if c.Files {
		out = append(out, hint(k.NextFile, "⇥/⇧⇥", "file"))
	}
	if c.Expand {
		out = append(out, k.Expand)
	}
	if c.Rail {
		out = append(out, k.ToggleRail)
	}
	return append(out, hint(k.NextTab, "[/]", "tab"), k.Back, Global.Help)
}

// FullHelp is the overlay. The form keys are on it as well: they are live only
// while a box has the keyboard, but the reader asking what tab does is owed
// both answers.
func (k DetailMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown},
		{k.NextTab, k.PrevTab, k.NextBlock, k.PrevBlock},
		{k.NextFile, k.PrevFile},
		{k.PaneLeft, k.PaneRight, k.FocusPane},
		{k.Expand, k.ToggleRail},
		{k.Reply, k.QuoteReply, k.React},
		{k.Edit, k.Delete, k.Resolve, k.Jump},
		{k.CopyLink, k.Browse},
		{k.Comment, k.Post, k.Activate, k.Editor},
		{Form.Next, Form.Prev, Form.Toggle},
		{k.Sync, k.Back, Global.Help, Global.Quit, Global.ForceQuit},
	}
}
