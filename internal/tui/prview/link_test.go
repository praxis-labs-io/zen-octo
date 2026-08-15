package prview_test

import (
	"strings"
	"testing"

	"github.com/zen-octo/zen-octo/internal/tui/prview"
)

// linked presses one of the two keys and reads back the URL it named.
func linked(t *testing.T, m prview.Model, k string) string {
	t.Helper()

	switch msg := asked(t, m, k).(type) {
	case prview.CopyLinkMsg:
		return msg.PR.URL
	case prview.BrowseMsg:
		return msg.PR.URL
	default:
		t.Fatalf("%q sent %T, want a link", k, msg)
		return ""
	}
}

// The keymap is the same on all four tabs, and so is what the two keys mean.
func TestCopyAndBrowseNameThePullRequestOnEveryTab(t *testing.T) {
	for _, k := range []string{"y", "O"} {
		t.Run(k, func(t *testing.T) {
			m := detailed(held(sampleDetail()), 200, 60)

			for tab := range 4 {
				if got := linked(t, m, k); got != sampleURL {
					t.Errorf("tab %d produced %q, want %q", tab, got, sampleURL)
				}
				m = press(m, "]")
			}
		})
	}
}

// Nothing under the pull request carries a URL, so a lit card must not change
// what either key copies or opens.
func TestCopyAndBrowseIgnoreTheRing(t *testing.T) {
	m := walked(detailed(held(sampleDetail()), 200, 60), 2)

	for _, k := range []string{"y", "O"} {
		if got := linked(t, m, k); got != sampleURL {
			t.Errorf("%q on a focused card produced %q, want the pull request", k, got)
		}
	}
}

// Both keys are letters. A leaked y writes nothing into the comment and a
// leaked O opens a browser over it, so a box has to take them as text.
func TestABoxTakesCopyAndBrowseAsText(t *testing.T) {
	boxes := map[string]prview.Model{
		"compose": composing(200, 60),
		"edit":    editing(tabComment),
	}

	for name, box := range boxes {
		t.Run(name, func(t *testing.T) {
			if got := stripANSI(typed(box, "yOu").View()); !strings.Contains(got, "yOu") {
				t.Error("the keys did not reach the box as text")
			}
		})
	}
}

// A picker and the merge form own the keyboard the same way, and the page
// behind them must not act on a key that was meant for the modal.
func TestAModalSwallowsCopyAndBrowse(t *testing.T) {
	modals := map[string]prview.Model{
		"picker": openPicker(t, "bug"),
		"merge":  openMerge(t),
	}

	for name, m := range modals {
		t.Run(name, func(t *testing.T) {
			for _, k := range []string{"y", "O"} {
				if got := asked(t, m, k); got != nil {
					t.Errorf("%q reached the screen behind the modal and sent %T", k, got)
				}
			}
		})
	}
}
