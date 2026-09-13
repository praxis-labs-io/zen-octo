package comp_test

import (
	"strings"
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
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
	if !strings.Contains(toasts.Render(testTheme), "Loaded 12 pull requests") {
		t.Error("Render() does not carry the text")
	}

	toasts.Expire(comp.ToastExpiredMsg{Seq: toasts.Seq()})
	if !toasts.Empty() {
		t.Error("Expire() left the toast showing")
	}
}

func TestAStaleTimerDoesNotClearANewerToast(t *testing.T) {
	var toasts comp.Toasts

	toasts.Show(comp.ToastInfo, "Refreshing")
	stale := comp.ToastExpiredMsg{Seq: toasts.Seq()}

	toasts.Show(comp.ToastError, "Refresh failed")
	toasts.Expire(stale)

	if toasts.Empty() {
		t.Fatal("the first toast's timer cleared the second")
	}
	if !strings.Contains(toasts.Render(testTheme), "Refresh failed") {
		t.Error("the wrong toast survived")
	}
}

func TestToastKindPicksTheColor(t *testing.T) {
	tests := []struct {
		name string
		kind comp.ToastKind
		want string
	}{
		{name: "success", kind: comp.ToastSuccess, want: fgSeq(testTheme.Success)},
		{name: "error", kind: comp.ToastError, want: fgSeq(testTheme.Error)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var toasts comp.Toasts
			toasts.Show(tt.kind, "message")

			if got := toasts.Render(testTheme); !strings.Contains(got, tt.want) {
				t.Errorf("Render() = %q, want the %s color", got, tt.name)
			}
		})
	}
}

func TestToastRendersNothingWhenEmpty(t *testing.T) {
	var toasts comp.Toasts

	if got := toasts.Render(testTheme); got != "" {
		t.Errorf("Render() = %q, want empty", got)
	}
}
