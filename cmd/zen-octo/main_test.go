package main

import (
	"io"
	"strings"
	"testing"
)

func TestRootRefusesBeforeTheTerminalIsTaken(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "two pull requests", args: []string{"acme/rocket#1", "acme/rocket#2"}, want: "accepts at most 1 arg"},
		{name: "not a pull request", args: []string{"acme/rocket"}, want: `no pull request in "acme/rocket"`},
		{name: "an issue URL", args: []string{"https://github.com/acme/rocket/issues/4"}, want: "no pull request in"},
		{name: "a pull request the mockup lacks", args: []string{"--mockup", "acme/rocket#1"}, want: "no pull request at acme/rocket#1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			cmd := newRootCmd()
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Execute(%v) error = %v, want it to contain %q", tt.args, err, tt.want)
			}
		})
	}
}
