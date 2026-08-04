package comp

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// ToastKind picks the color a toast carries.
type ToastKind int

const (
	ToastInfo ToastKind = iota
	ToastSuccess
	ToastError
)

const toastTTL = 4 * time.Second

// ToastExpiredMsg retires a toast.
type ToastExpiredMsg struct{ Seq int }

// Toasts holds the one message currently on the status bar. Writes are
// optimistic across the app, so this is how a result reaches the user without
// taking over the frame.
type Toasts struct {
	text string
	kind ToastKind
	seq  int
}

// Show replaces whatever is showing and returns the command that retires it.
// The sequence number is what keeps a stale timer from clearing the toast that
// replaced the one it was fired for.
func (t *Toasts) Show(kind ToastKind, text string) tea.Cmd {
	t.seq++
	t.text, t.kind = text, kind

	seq := t.seq
	return tea.Tick(toastTTL, func(time.Time) tea.Msg { return ToastExpiredMsg{Seq: seq} })
}

// Expire clears the toast the message was fired for and ignores any other.
func (t *Toasts) Expire(msg ToastExpiredMsg) {
	if msg.Seq == t.seq {
		t.text = ""
	}
}

// Seq identifies the toast currently showing. Anything holding a Toasts and
// reasoning about a ToastExpiredMsg needs it to tell current from stale.
func (t Toasts) Seq() int { return t.seq }

// Empty reports whether anything is showing, so the status bar knows when to
// fall back to the keymap hints.
func (t Toasts) Empty() bool { return t.text == "" }

// Render styles the current toast.
func (t Toasts) Render(th theme.Theme) string {
	if t.text == "" {
		return ""
	}

	c := th.Primary
	switch t.kind {
	case ToastSuccess:
		c = th.Success
	case ToastError:
		c = th.Error
	case ToastInfo:
		c = th.Primary
	}
	return lipgloss.NewStyle().Foreground(c).Render(t.text)
}
