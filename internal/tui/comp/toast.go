package comp

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

type ToastKind int

const (
	ToastInfo ToastKind = iota
	ToastSuccess
	ToastError
)

const toastTTL = 4 * time.Second

// ToastExpiredMsg retires the toast numbered Seq.
type ToastExpiredMsg struct{ Seq int }

// Toasts holds the one message currently on the status bar.
type Toasts struct {
	text string
	kind ToastKind
	seq  int
}

// Show replaces whatever is showing and returns the command that retires it.
func (t *Toasts) Show(kind ToastKind, text string) tea.Cmd {
	t.seq++
	t.text, t.kind = text, kind

	seq := t.seq
	return tea.Tick(toastTTL, func(time.Time) tea.Msg { return ToastExpiredMsg{Seq: seq} })
}

// Expire clears the toast msg was fired for and ignores any other.
func (t *Toasts) Expire(msg ToastExpiredMsg) {
	if msg.Seq == t.seq {
		t.text = ""
	}
}

func (t Toasts) Seq() int { return t.seq }

func (t Toasts) Empty() bool { return t.text == "" }

func (t Toasts) Render(th theme.Theme) string {
	if t.text == "" {
		return ""
	}

	c := th.Text
	switch t.kind {
	case ToastSuccess:
		c = th.Success
	case ToastError:
		c = th.Error
	case ToastInfo:
		c = th.Text
	}
	return lipgloss.NewStyle().Foreground(c).Render(t.text)
}
