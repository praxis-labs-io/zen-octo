// Package keys declares every key binding once, with the help text the status bar and overlay render from.
package keys

import "charm.land/bubbles/v2/key"

// GlobalMap is handled by the root model before any screen sees the key.
type GlobalMap struct {
	Help key.Binding
	Quit key.Binding
	// ForceQuit is separate from Quit because a compose box takes q as text.
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

	CopyLink key.Binding
	Browse   key.Binding

	Search key.Binding
	// ClearSearch drops a standing filter whether or not the search bar is open.
	ClearSearch key.Binding
}

// DetailMap is live on the pull request detail screen; focus decides what the movement keys move.
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

	// NextInColumn and PrevInColumn step the file, commit, or check column driving the pane.
	NextInColumn key.Binding
	PrevInColumn key.Binding
	ToggleViewed key.Binding

	NextBlock key.Binding
	PrevBlock key.Binding

	PaneLeft  key.Binding
	PaneRight key.Binding

	SplitView key.Binding

	// FocusPane covers every digit, numbering panes by where they sit.
	FocusPane key.Binding

	ToggleRail key.Binding
	Expand     key.Binding
	Sync       key.Binding
	Back       key.Binding

	Comment key.Binding
	// Post is a Kitty keyboard protocol chord; other terminals reach the compose button instead.
	Post     key.Binding
	Activate key.Binding
	Editor   key.Binding

	Reply      key.Binding
	QuoteReply key.Binding

	// React toggles one of GitHub's eight reactions on the block the ring is on.
	React key.Binding

	Edit   key.Binding
	Delete key.Binding

	// Resolve both resolves and unresolves a review thread.
	Resolve key.Binding
	Jump    key.Binding

	// Search, NextMatch, and PrevMatch search within a job log.
	Search       key.Binding
	NextMatch    key.Binding
	PrevMatch    key.Binding
	FirstFailure key.Binding

	CopyLink key.Binding
	Browse   key.Binding
}

// FormMap is live while a compose box, the merge form, or a picker holds the keyboard.
type FormMap struct {
	Next key.Binding
	Prev key.Binding

	Toggle key.Binding
}

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
		NextSection:  key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next tab")),
		PrevSection:  key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev tab")),
		Open:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open")),
		Sync:         key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sync")),
		CopyLink:     key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy link")),
		Browse:       key.NewBinding(key.WithKeys("O"), key.WithHelp("O", "open in browser")),
		Search:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		ClearSearch:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear search")),
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
		NextInColumn: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next in the column")),
		PrevInColumn: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "previous in the column")),
		ToggleViewed: key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mark viewed")),
		NextBlock:    key.NewBinding(key.WithKeys("}"), key.WithHelp("}", "next block")),
		PrevBlock:    key.NewBinding(key.WithKeys("{"), key.WithHelp("{", "prev block")),
		PaneLeft:     key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h/←", "pane left")),
		PaneRight:    key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/→", "pane right")),
		SplitView:    key.NewBinding(key.WithKeys("|"), key.WithHelp("|", "side by side")),
		FocusPane:    key.NewBinding(key.WithKeys("1", "2", "3"), key.WithHelp("1/2/3", "focus pane")),
		ToggleRail:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "details")),
		Expand:       key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "expand")),
		Sync:         key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sync")),
		Back:         key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),

		Comment:      key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "comment")),
		Post:         key.NewBinding(key.WithKeys("ctrl+enter"), key.WithHelp("ctrl+⏎", "post")),
		Activate:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open or press")),
		Editor:       key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "$EDITOR")),
		Reply:        key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reply or rerun")),
		QuoteReply:   key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "quote reply or rerun all")),
		React:        key.NewBinding(key.WithKeys("+"), key.WithHelp("+", "react")),
		Edit:         key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		Delete:       key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "delete")),
		Resolve:      key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "resolve or unresolve")),
		Jump:         key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "show in the diff")),
		Search:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search log")),
		NextMatch:    key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match")),
		PrevMatch:    key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "previous match")),
		FirstFailure: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "first failure")),
		CopyLink:     key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy link")),
		Browse:       key.NewBinding(key.WithKeys("O"), key.WithHelp("O", "open in browser")),
	}

	Form = FormMap{
		Next:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
		Prev:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev field")),
		Toggle: key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "check or uncheck")),
	}
)

func hint(b key.Binding, keys, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(b.Keys()...), key.WithHelp(keys, desc))
}

// ListContext says what the list screen can act on, so its hints name only keys that answer.
type ListContext struct {
	// Rows is whether the section's rows are drawn rather than a spinner or error standing in.
	Rows bool

	// Search is whether a filter is standing.
	Search bool
}

// ShortHelp is the status bar line for c. It omits Quit and ends in Help, which the bar never sheds.
func (k ListMap) ShortHelp(c ListContext) []key.Binding {
	out := make([]key.Binding, 0, 9)
	if c.Rows {
		out = append(out, hint(k.Down, "j/k", "move"), k.Open)
	}
	out = append(out, hint(k.NextSection, "[/]", "tab"))
	if c.Rows {
		out = append(out, k.Search)
	}
	if c.Search {
		out = append(out, k.ClearSearch)
	}
	if c.Rows {
		out = append(out, k.CopyLink, hint(k.Browse, "O", "browser"))
	}
	return append(out, k.Sync, Global.Help)
}

// SearchHelp is the status bar line while the search bar has the keyboard.
func (k ListMap) SearchHelp() []key.Binding {
	return []key.Binding{
		hint(k.Open, "⏎", "apply"),
		hint(k.ClearSearch, "esc", "clear"),
	}
}

func (k ListMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown},
		{k.NextSection, k.PrevSection, k.Open, k.Sync},
		{k.CopyLink, k.Browse},
		{k.Search, k.ClearSearch},
		{Global.Help, Global.Quit, Global.ForceQuit},
	}
}

// DetailContext says what the detail screen can act on, so its hints name only keys that answer.
type DetailContext struct {
	// Blocks is whether the braces have anything to walk.
	Blocks bool

	Expand bool

	// Rail is whether the tab has room for a rail to toggle.
	Rail bool

	// Activate is whether the focused pane opens rows on enter, whatever row is under the cursor.
	Activate bool

	// Panes is whether to name the step between panes, which only the rail wants.
	Panes bool

	// Column names what the driving column holds, or is empty where the tab has none.
	Column string

	// Split is whether the pane draws a diff that can be split into two columns.
	Split bool

	// FileView is whether a file is under the cursor; FileViewed picks which action is named.
	FileView   bool
	FileViewed bool

	JobLog     bool
	JobFailure bool
	JobMatches bool
	JobRerun   bool

	// RunRerun and RunRerunAll are set only on a workflow row.
	RunRerun    bool
	RunRerunAll bool

	// SearchStanding is whether a settled query still filters the job log.
	SearchStanding bool
}

// ShortHelp is the status bar line for c. Sync is left to the overlay.
func (k DetailMap) ShortHelp(c DetailContext) []key.Binding {
	out := []key.Binding{hint(k.Down, "j/k", "move")}
	if c.Activate {
		out = append(out, hint(k.Activate, "⏎", "open"))
	}
	if c.Panes {
		out = append(out, hint(k.PaneRight, "h/l", "panes"))
	}

	back := k.Back
	if c.SearchStanding {
		back = hint(k.Back, "esc", "clear search")
	}
	out = append(out, back, hint(k.NextTab, "[/]", "tab"))
	if c.Rail {
		out = append(out, k.ToggleRail)
	}

	if c.Blocks {
		out = append(out, hint(k.NextBlock, "{/}", "block"))
	}
	if c.Column != "" {
		out = append(out, hint(k.NextInColumn, "⇥/⇧⇥", c.Column))
	}
	if c.Expand {
		out = append(out, k.Expand)
	}
	if c.FileView {
		action := "mark viewed"
		if c.FileViewed {
			action = "unmark viewed"
		}
		out = append(out, hint(k.ToggleViewed, "m", action))
	}
	if c.Split {
		out = append(out, k.SplitView)
	}
	if c.JobLog {
		out = append(out, k.Search)
	}
	if c.JobFailure {
		out = append(out, k.FirstFailure)
	}
	if c.JobMatches {
		out = append(out, hint(k.NextMatch, "n/N", "match"))
	}
	if c.JobRerun {
		out = append(out, hint(k.Reply, "r", "rerun"))
	}
	if c.RunRerun {
		out = append(out, hint(k.Reply, "r", "rerun failed"))
	}
	if c.RunRerunAll {
		out = append(out, hint(k.QuoteReply, "R", "rerun all"))
	}
	return append(out, Global.Help)
}

// SearchHelp is the status bar line while the job log's search bar has the keyboard.
func (k DetailMap) SearchHelp() []key.Binding {
	return []key.Binding{
		hint(k.Activate, "⏎", "apply"),
		hint(k.Back, "esc", "clear"),
	}
}

// FullHelp is the help overlay, form keys included.
func (k DetailMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown},
		{k.NextTab, k.PrevTab, k.NextBlock, k.PrevBlock},
		{k.NextInColumn, k.PrevInColumn, k.ToggleViewed},
		{k.PaneLeft, k.PaneRight, k.FocusPane, k.SplitView},
		{k.Expand, k.ToggleRail},
		{k.Reply, k.QuoteReply, k.React},
		{k.Edit, k.Delete, k.Resolve, k.Jump},
		{k.Search, k.NextMatch, k.PrevMatch, k.FirstFailure},
		{k.CopyLink, k.Browse},
		{k.Comment, k.Post, k.Activate, k.Editor},
		{Form.Next, Form.Prev, Form.Toggle},
		{k.Sync, k.Back, Global.Help, Global.Quit, Global.ForceQuit},
	}
}
