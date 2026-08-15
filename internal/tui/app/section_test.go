package app_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/zen-octo/zen-octo/internal/config"
	"github.com/zen-octo/zen-octo/internal/tui/app"
	"github.com/zen-octo/zen-octo/internal/tui/list"
)

// recentConfig is one section whose filter names a window rather than a date.
func recentConfig() *config.Config {
	return &config.Config{
		PRSections: []config.Section{
			{Title: "Recently Closed", Filters: "is:pr is:closed author:@me closed:>={{since:24h}}"},
		},
		Defaults: config.Defaults{PRsLimit: 20, IssuesLimit: 20},
		Theme:    "rose-pine-moon",
	}
}

// boundIn reads the closed:>= instant out of a query the client was handed.
func boundIn(t *testing.T, query string) time.Time {
	t.Helper()

	_, after, ok := strings.Cut(query, "closed:>=")
	if !ok {
		t.Fatalf("query %q carries no closed bound", query)
	}
	stamp, _, _ := strings.Cut(after, " ")
	at, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		t.Fatalf("bound %q in %q is not RFC 3339: %v", stamp, query, err)
	}
	return at
}

// GitHub takes an absolute instant and nothing relative, so the token has to be
// gone by the time the query goes out, and the bound has to be the real one.
func TestASectionsWindowReachesTheClientAsATimestamp(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	before := time.Now().UTC()

	drive(t, app.New(recentConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})

	if len(client.queries) == 0 {
		t.Fatal("the section was never fetched")
	}
	query := client.queries[0]
	if strings.Contains(query, "{{") {
		t.Fatalf("query = %q, want the token rendered before the search", query)
	}

	got := boundIn(t, query)
	if want := before.Add(-24 * time.Hour); got.Before(want.Add(-2*time.Second)) || got.After(time.Now().UTC()) {
		t.Errorf("bound = %s, want a day before now (%s)", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

// Every fetch renders its own bound, so a session left open keeps asking about
// the day behind it. The clock cannot be moved here, so this is the weaker half.
func TestEveryFetchRendersItsOwnWindow(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := drive(t, app.New(recentConfig(), client), tea.WindowSizeMsg{Width: 120, Height: 40})
	settle(m, list.RefreshMsg{})

	if len(client.queries) < 2 {
		t.Fatalf("the client saw %d queries, want the refresh to have fetched again", len(client.queries))
	}
	for i, query := range client.queries {
		if strings.Contains(query, "{{") {
			t.Fatalf("query %d = %q, want every fetch to render the token", i, query)
		}
	}

	first, second := boundIn(t, client.queries[0]), boundIn(t, client.queries[len(client.queries)-1])
	if second.Before(first) {
		t.Errorf("the refresh asked about %s, older than the first fetch's %s",
			second.Format(time.RFC3339), first.Format(time.RFC3339))
	}
}
