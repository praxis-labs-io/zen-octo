package app_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/config"
	"github.com/zen-octo/zen-octo/internal/gh"
	"github.com/zen-octo/zen-octo/internal/tui/app"
	"github.com/zen-octo/zen-octo/internal/tui/theme"
)

// fakeSearcher answers the one call the root model makes.
type fakeSearcher struct {
	prs []gh.PullRequest
	err error

	gotQuery string
	gotLimit int
	calls    int
}

func (f *fakeSearcher) SearchPullRequests(_ context.Context, query string, limit int) ([]gh.PullRequest, error) {
	f.calls++
	f.gotQuery, f.gotLimit = query, limit
	return f.prs, f.err
}

func testConfig() *config.Config {
	return &config.Config{
		PRSections: []config.Section{{Title: "My PRs", Filters: "is:open author:@me"}},
		Defaults:   config.Defaults{PRsLimit: 20, IssuesLimit: 20},
		Theme:      "rose-pine-moon",
	}
}

func samplePRs() []gh.PullRequest {
	return []gh.PullRequest{
		{
			Number: 412, Title: "Fix auth retry", Repository: "zen-octo/zen-octo",
			Author: gh.Actor{Login: "drucial"}, State: gh.PRStateOpen,
			Checks: gh.CheckStateSuccess, UpdatedAt: time.Now().Add(-2 * time.Hour),
		},
		{
			Number: 408, Title: "Bump deps", Repository: "zen-octo/zen-octo",
			Author: gh.Actor{Login: "drucial"}, State: gh.PRStateOpen, IsDraft: true,
			Checks: gh.CheckStateFailure, UpdatedAt: time.Now().Add(-30 * time.Hour),
		},
	}
}

// drive runs the model's Init command, feeds every resulting message back, then
// applies the given messages in order. It returns the settled model.
func drive(t *testing.T, m tea.Model, msgs ...tea.Msg) tea.Model {
	t.Helper()

	for _, msg := range flatten(m.Init()) {
		m, _ = m.Update(msg)
	}
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m
}

// flatten runs a command and returns every message it produces, unpacking
// batches. Init batches the spinner tick alongside the fetch, so taking only
// the first message would drop the one that matters.
func flatten(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, sub := range batch {
			out = append(out, flatten(sub)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

func render(t *testing.T, m tea.Model) string {
	t.Helper()
	return m.View().Content
}

func TestRendersFetchedPullRequests(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})

	out := render(t, m)
	for _, want := range []string{"My PRs", "#412", "Fix auth retry", "zen-octo/zen-octo", "drucial", "#408"} {
		if !strings.Contains(out, want) {
			t.Errorf("view is missing %q\n%s", want, out)
		}
	}
}

func TestPassesSectionQueryAndLimitToTheClient(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	drive(t, app.New(testConfig(), client))

	if client.gotQuery != "is:open author:@me" {
		t.Errorf("query = %q, want the section filters unmodified", client.gotQuery)
	}
	if client.gotLimit != 20 {
		t.Errorf("limit = %d, want 20", client.gotLimit)
	}
}

func TestRendersEmptySectionWithoutClaimingAnError(t *testing.T) {
	client := &fakeSearcher{prs: nil}
	m := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})

	out := render(t, m)
	if !strings.Contains(out, "Nothing matches this section.") {
		t.Errorf("view = %q, want the empty-section message", out)
	}
	if strings.Contains(out, "Failed to load") {
		t.Error("view claims a failure for an empty result")
	}
}

func TestRendersTheFixCommandWhenAScopeIsMissing(t *testing.T) {
	client := &fakeSearcher{err: errors.New("HTTP 403\nYour gh token is missing the workflow scope. Run:\n  gh auth refresh -s workflow")}
	m := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})

	out := render(t, m)
	if !strings.Contains(out, "Failed to load") {
		t.Errorf("view = %q, want the failure label", out)
	}
	if !strings.Contains(out, "gh auth refresh -s workflow") {
		t.Errorf("view = %q, want the fix command carried through to the screen", out)
	}
}

func TestCursorMovesAndStopsAtTheEnds(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	base := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})

	// Two rows, so one "j" lands on the second and a second "j" holds there.
	moved := applyKeys(base, "j", "j")
	if !strings.Contains(selectedLine(t, moved), "#408") {
		t.Errorf("selection = %q, want it clamped to the last row", selectedLine(t, moved))
	}

	back := applyKeys(moved, "k", "k", "k")
	if !strings.Contains(selectedLine(t, back), "#412") {
		t.Errorf("selection = %q, want it clamped to the first row", selectedLine(t, back))
	}
}

func TestRefreshRefetches(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := drive(t, app.New(testConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})

	if client.calls != 1 {
		t.Fatalf("calls = %d after load, want 1", client.calls)
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("pressing r produced no command, want a refetch")
	}
	flatten(cmd)

	if client.calls != 2 {
		t.Errorf("calls = %d after refresh, want 2", client.calls)
	}
}

func TestQuitKeys(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		{Code: 'q', Text: "q"},
		{Code: 'c', Mod: tea.ModCtrl},
	} {
		client := &fakeSearcher{prs: samplePRs()}
		m := drive(t, app.New(testConfig(), client))

		_, cmd := m.Update(key)
		if cmd == nil {
			t.Fatalf("%q produced no command, want quit", key.String())
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%q did not produce a QuitMsg", key.String())
		}
	}
}

func applyKeys(m tea.Model, keys ...string) tea.Model {
	for _, k := range keys {
		m, _ = m.Update(tea.KeyPressMsg{Code: rune(k[0]), Text: k})
	}
	return m
}

// selectedLine returns the rendered row painted with the selection background.
// Matching the exact color keeps this from picking up the header, which has a
// background of its own.
func selectedLine(t *testing.T, m tea.Model) string {
	t.Helper()

	r, g, b, _ := theme.RosePineMoon.SelectedBackground.RGBA()
	want := fmt.Sprintf("48;2;%d;%d;%d", r>>8, g>>8, b>>8)

	for _, line := range strings.Split(render(t, m), "\n") {
		if strings.Contains(line, want) {
			return line
		}
	}
	return ""
}
