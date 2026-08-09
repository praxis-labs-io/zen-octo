package prview

// The one test in this package rather than beside it. editorCommand reads the
// environment and returns a command line; there is no frame it reaches, and
// exporting it so a black-box test could call it would be widening the package
// for the test's convenience.

import (
	"slices"
	"testing"
)

// The editor is the reader's, in the order every other terminal program reads
// them, and vi when they have named none.
func TestTheEditorIsTheOneTheReaderNamed(t *testing.T) {
	tests := []struct {
		name     string
		visual   string
		editor   string
		wantName string
		wantArgs []string
	}{
		{name: "neither set", wantName: "vi"},
		{name: "EDITOR", editor: "nvim", wantName: "nvim"},
		{name: "VISUAL wins", visual: "hx", editor: "nvim", wantName: "hx"},
		{name: "arguments come with it", editor: "code -w", wantName: "code", wantArgs: []string{"-w"}},
		{name: "blank is unset", editor: "   ", wantName: "vi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("VISUAL", tt.visual)
			t.Setenv("EDITOR", tt.editor)

			name, args := editorCommand()
			if name != tt.wantName {
				t.Errorf("editor = %q, want %q", name, tt.wantName)
			}
			if !slices.Equal(args, tt.wantArgs) {
				t.Errorf("args = %q, want %q", args, tt.wantArgs)
			}
		})
	}
}
