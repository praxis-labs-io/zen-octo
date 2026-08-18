package prview_test

import (
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/golden"
)

// TestGoldenFilesTab locks the Files tab where its width proves something. The
// frames are stripped, so they hold alignment and clipping and never a fill.
func TestGoldenFilesTab(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		keys          []string
	}{
		{name: "files-open", width: 100, height: 30},

		// One under treeMinFrame, the only width that proves the tab draws with
		// no column beside it.
		{name: "files-no-tree", width: 69, height: 24},

		// Narrow enough that a nested path outruns the column and a line of code
		// outruns the diff, which proves both clip before they draw.
		{name: "files-clipping", width: 74, height: 24},

		// A folded directory, and the file being read gone with it, so the pane
		// has had to find another.
		{name: "files-folded", width: 100, height: 30, keys: []string{"1", "k", "space"}},

		// A file GitHub returned no body for, which is a heading and a reason.
		{name: "files-omitted", width: 100, height: 30, keys: []string{"1", "g", "j"}},

		// A card lit, so the frame holds the footer the ring gives it and the
		// keys that footer names.
		{name: "files-lit", width: 100, height: 30, keys: []string{"}", "}"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := press(onFiles(tt.width, tt.height), tt.keys...)
			golden.Compare(t, tt.name, []byte(stripANSI(m.View())+"\n"))
		})
	}
}
