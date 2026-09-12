package prview_test

import (
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/golden"
)

func TestGoldenFilesTab(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		keys          []string
	}{
		{name: "files-open", width: 100, height: 30},

		{name: "files-no-tree", width: 69, height: 24},

		{name: "files-clipping", width: 74, height: 24},

		{name: "files-folded", width: 100, height: 30, keys: []string{"1", "k", "space"}},

		{name: "files-omitted", width: 100, height: 30, keys: []string{"1", "g", "j"}},

		{name: "files-lit", width: 100, height: 30, keys: []string{"}", "}"}},

		{name: "files-cursor", width: 100, height: 30, keys: []string{"}", "j", "j"}},

		{name: "files-split", width: 140, height: 30, keys: []string{"|"}},

		{name: "files-split-base", width: 140, height: 30, keys: []string{"|", "}", "j", "h"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := press(onFiles(tt.width, tt.height), tt.keys...)
			golden.Compare(t, tt.name, []byte(stripANSI(m.View())+"\n"))
		})
	}
}
