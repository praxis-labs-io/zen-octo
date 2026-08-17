package prview_test

import (
	"testing"

	"github.com/zen-octo/zen-octo/internal/golden"
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

		// A folded directory, and the file under it gone with it.
		{name: "files-folded", width: 100, height: 30, keys: []string{"1", "o"}},

		// A folded file, which keeps its heading and drops its diff.
		{name: "files-folded-file", width: 100, height: 30, keys: []string{"1", "j", "o"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := press(onFiles(tt.width, tt.height), tt.keys...)
			golden.Compare(t, tt.name, []byte(stripANSI(m.View())+"\n"))
		})
	}
}
