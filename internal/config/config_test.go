package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zen-octo/zen-octo/internal/config"
)

// writeConfig points config.Dir at a temp dir and writes body to config.yml.
// Passing an empty body leaves the directory without a config file.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(config.DirEnv, dir)
	if body != "" {
		path := filepath.Join(dir, "config.yml")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("writing config: %v", err)
		}
	}
	return dir
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	writeConfig(t, "")

	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	want := config.Default()
	if len(got.PRSections) != len(want.PRSections) {
		t.Errorf("PRSections = %d, want %d", len(got.PRSections), len(want.PRSections))
	}
	if got.Theme != want.Theme {
		t.Errorf("Theme = %q, want %q", got.Theme, want.Theme)
	}
	if got.Defaults.PRsLimit != want.Defaults.PRsLimit {
		t.Errorf("PRsLimit = %d, want %d", got.Defaults.PRsLimit, want.Defaults.PRsLimit)
	}
}

func TestLoadKeepsFileValuesAndFillsTheRest(t *testing.T) {
	writeConfig(t, `
prSections:
  - title: Mine
    filters: is:open author:@me
defaults:
  prsLimit: 5
`)

	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if len(got.PRSections) != 1 || got.PRSections[0].Title != "Mine" {
		t.Errorf("PRSections = %+v, want one section titled Mine", got.PRSections)
	}
	if got.Defaults.PRsLimit != 5 {
		t.Errorf("PRsLimit = %d, want 5", got.Defaults.PRsLimit)
	}
	// Absent from the file, so defaults fill in.
	if got.Defaults.IssuesLimit != 20 {
		t.Errorf("IssuesLimit = %d, want 20", got.Defaults.IssuesLimit)
	}
	if got.Theme != "rose-pine-moon" {
		t.Errorf("Theme = %q, want rose-pine-moon", got.Theme)
	}
	if len(got.IssueSections) == 0 {
		t.Error("IssueSections is empty, want defaults")
	}
}

// The syntax palette stays empty unless it is asked for. A theme already names
// the Chroma style that matches it, and filling one in here would override
// every theme with the default's.
func TestSyntaxThemeIsEmptyUntilItIsSet(t *testing.T) {
	writeConfig(t, "theme: rose-pine-moon\n")

	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if got.SyntaxTheme != "" {
		t.Errorf("SyntaxTheme = %q, want it left to the theme", got.SyntaxTheme)
	}
}

func TestSyntaxThemeIsReadFromTheFile(t *testing.T) {
	writeConfig(t, "theme: rose-pine-moon\nsyntaxTheme: tokyonight-moon\n")

	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if got.SyntaxTheme != "tokyonight-moon" {
		t.Errorf("SyntaxTheme = %q, want tokyonight-moon", got.SyntaxTheme)
	}
}

func TestLoadRejectsInvalidConfigs(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantText string
	}{
		{
			name:     "section without title",
			body:     "prSections:\n  - filters: is:open\n",
			wantText: "prSections[0]: title is required",
		},
		{
			name:     "section without filters",
			body:     "prSections:\n  - title: Mine\n",
			wantText: `prSections[0] ("Mine"): filters is required`,
		},
		{
			name:     "issue section without filters",
			body:     "issueSections:\n  - title: Bugs\n",
			wantText: `issueSections[0] ("Bugs"): filters is required`,
		},
		{
			name:     "limit above the ceiling",
			body:     "defaults:\n  prsLimit: 500\n",
			wantText: "prsLimit: 500 is out of range",
		},
		{
			name:     "negative limit",
			body:     "defaults:\n  issuesLimit: -1\n",
			wantText: "issuesLimit: -1 is out of range",
		},
		{
			name:     "malformed yaml",
			body:     "prSections: [oh dear\n",
			wantText: "parsing",
		},
		{
			name:     "since token that is not a duration",
			body:     "prSections:\n  - title: Recent\n    filters: 'is:pr closed:>={{since:one day}}'\n",
			wantText: `prSections[0] ("Recent"): "one day" is not a duration`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeConfig(t, tt.body)

			_, err := config.Load()
			if err == nil {
				t.Fatal("Load() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("Load() error = %q, want it to contain %q", err, tt.wantText)
			}
		})
	}
}

func TestDefaultSectionsQualifyTheSearchType(t *testing.T) {
	// GitHub searches issues and pull requests from one index. A section
	// without is:pr or is:issue spends its limit on the wrong kind, and the
	// caller silently drops what it didn't want.
	cfg := config.Default()

	for _, s := range cfg.PRSections {
		if !strings.Contains(s.Filters, "is:pr") {
			t.Errorf("PR section %q has filters %q, want is:pr", s.Title, s.Filters)
		}
	}
	for _, s := range cfg.IssueSections {
		if !strings.Contains(s.Filters, "is:issue") {
			t.Errorf("issue section %q has filters %q, want is:issue", s.Title, s.Filters)
		}
	}
}

// GitHub's date qualifiers take an absolute instant and nothing relative, so
// the filter carries a token and the client renders it before the search.
func TestExpandQueryRendersTheSinceToken(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		filters string
		want    string
	}{
		{
			name:    "a day back",
			filters: "is:pr is:closed closed:>={{since:24h}}",
			want:    "is:pr is:closed closed:>=2026-08-14T12:00:00Z",
		},
		{
			name:    "no token passes through untouched",
			filters: "is:open is:pr author:@me",
			want:    "is:open is:pr author:@me",
		},
		{
			name:    "two tokens both render",
			filters: "closed:{{since:48h}}..{{since:24h}}",
			want:    "closed:2026-08-13T12:00:00Z..2026-08-14T12:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := config.ExpandQuery(tt.filters, now); got != tt.want {
				t.Errorf("ExpandQuery() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A bound written in the reader's zone would move the window by hours twice a
// year, so the instant is always UTC whatever clock now came off.
func TestExpandQueryWritesTheBoundInUTC(t *testing.T) {
	zone := time.FixedZone("well east", 11*60*60)
	now := time.Date(2026, 8, 15, 6, 0, 0, 0, zone)

	want := "closed:>=2026-08-13T19:00:00Z"
	if got := config.ExpandQuery("closed:>={{since:24h}}", now); got != want {
		t.Errorf("ExpandQuery() = %q, want %q", got, want)
	}
}

// The shipped defaults never reach validate: Load answers with them before it
// runs, so a token typo in one of them would only surface as a failed search.
func TestDefaultSectionsExpandCleanly(t *testing.T) {
	cfg := config.Default()

	for _, s := range append(append([]config.Section{}, cfg.PRSections...), cfg.IssueSections...) {
		if got := config.ExpandQuery(s.Filters, time.Now()); strings.Contains(got, "{{") {
			t.Errorf("section %q expands to %q, want no token left in it", s.Title, got)
		}
	}
}

func TestPathSitsInsideDir(t *testing.T) {
	dir := writeConfig(t, "")

	got, err := config.Path()
	if err != nil {
		t.Fatalf("Path() error = %v, want nil", err)
	}
	if want := filepath.Join(dir, "config.yml"); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}
