package prview_test

import (
	"strings"
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

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

func TestCopyAndBrowseIgnoreTheRing(t *testing.T) {
	m := walked(detailed(held(sampleDetail()), 200, 60), 2)

	for _, k := range []string{"y", "O"} {
		if got := linked(t, m, k); got != sampleURL {
			t.Errorf("%q on a focused card produced %q, want the pull request", k, got)
		}
	}
}

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
