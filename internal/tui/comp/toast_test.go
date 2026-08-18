package comp_test

import (
	"strings"
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

func TestToastShowsThenExpires(t *testing.T) {
	var toasts comp.Toasts

	if !toasts.Empty() {
		t.Fatal("a fresh Toasts is not empty")
	}

	if cmd := toasts.Show(comp.ToastSuccess, "Loaded 12 pull requests"); cmd == nil {
		t.Fatal("Show() returned no command, so nothing will ever retire the toast")
	}
	if toasts.Empty() {
		t.Fatal("Show() left nothing showing")
	}
	if !strings.Contains(toasts.Render(theme.RosePineMoon), "Loaded 12 pull requests") {
		t.Error("Render() does not carry the text")
	}

	toasts.Expire(comp.ToastExpiredMsg{Seq: toasts.Seq()})
	if !toasts.Empty() {
		t.Error("Expire() left the toast showing")
	}
}

// Two toasts in quick succession leave the first one's timer still in flight.
// Without the sequence number it lands and clears the second.
func TestAStaleTimerDoesNotClearANewerToast(t *testing.T) {
	var toasts comp.Toasts

	toasts.Show(comp.ToastInfo, "Refreshing")
	stale := comp.ToastExpiredMsg{Seq: toasts.Seq()}

	toasts.Show(comp.ToastError, "Refresh failed")
	toasts.Expire(stale)

	if toasts.Empty() {
		t.Fatal("the first toast's timer cleared the second")
	}
	if !strings.Contains(toasts.Render(theme.RosePineMoon), "Refresh failed") {
		t.Error("the wrong toast survived")
	}
}

func TestToastKindPicksTheColor(t *testing.T) {
	tests := []struct {
		name string
		kind comp.ToastKind
		want string
	}{
		{name: "info", kind: comp.ToastInfo, want: fgSeq(theme.RosePineMoon.Text)},
		{name: "success", kind: comp.ToastSuccess, want: fgSeq(theme.RosePineMoon.Success)},
		{name: "error", kind: comp.ToastError, want: fgSeq(theme.RosePineMoon.Error)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var toasts comp.Toasts
			toasts.Show(tt.kind, "message")

			if got := toasts.Render(theme.RosePineMoon); !strings.Contains(got, tt.want) {
				t.Errorf("Render() = %q, want the %s color", got, tt.name)
			}
		})
	}
}

func TestToastRendersNothingWhenEmpty(t *testing.T) {
	var toasts comp.Toasts

	if got := toasts.Render(theme.RosePineMoon); got != "" {
		t.Errorf("Render() = %q, want empty", got)
	}
}
