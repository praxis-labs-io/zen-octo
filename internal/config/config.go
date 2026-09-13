// Package config loads zen-octo's YAML configuration from ~/.zen-octo.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/goccy/go-yaml"
)

// DirEnv names the environment variable that overrides the config directory.
const DirEnv = "ZEN_OCTO_CONFIG_DIR"

const (
	dirName  = ".zen-octo"
	fileName = "config.yml"

	maxLimit = 100
)

// Section is one tab in the list view: a title and a raw GitHub search query.
type Section struct {
	Title   string `yaml:"title"`
	Filters string `yaml:"filters"`
}

var sinceToken = regexp.MustCompile(`\{\{since:([^}]*)\}\}`)

// ExpandQuery replaces each {{since:DURATION}} in filters with the RFC 3339 UTC
// instant that long before now, leaving a token whose duration does not parse as written.
func ExpandQuery(filters string, now time.Time) string {
	return sinceToken.ReplaceAllStringFunc(filters, func(token string) string {
		d, err := time.ParseDuration(sinceToken.FindStringSubmatch(token)[1])
		if err != nil {
			return token
		}
		return now.UTC().Add(-d).Format(time.RFC3339)
	})
}

type Defaults struct {
	PRsLimit    int `yaml:"prsLimit"`
	IssuesLimit int `yaml:"issuesLimit"`
}

// Config is the config file after defaults are applied.
type Config struct {
	PRSections    []Section `yaml:"prSections"`
	IssueSections []Section `yaml:"issueSections"`
	Defaults      Defaults  `yaml:"defaults"`
	// Theme overrides individual colors of the theme derived from the terminal.
	Theme Theme `yaml:"theme"`
	// Transparent withholds the painted background and nothing else.
	Transparent bool `yaml:"transparent"`
	// SyntaxTheme names the Chroma style for code. Empty pairs one against the background.
	SyntaxTheme string `yaml:"syntaxTheme"`
	UpdateCheck *bool  `yaml:"updateCheck"`
}

// ChecksForUpdates reports updateCheck, which is on when the key is absent.
func (c *Config) ChecksForUpdates() bool {
	return c.UpdateCheck == nil || *c.UpdateCheck
}

// Default is the config used when no file exists.
func Default() *Config {
	return &Config{
		PRSections: []Section{
			{Title: "My PRs", Filters: "is:open is:pr author:@me"},
			{Title: "Needs My Review", Filters: "is:open is:pr review-requested:@me"},
			{Title: "Involved", Filters: "is:open is:pr involves:@me -author:@me"},
			{Title: "Recently Closed", Filters: "is:pr is:closed author:@me closed:>={{since:24h}} sort:updated-desc"},
		},
		IssueSections: []Section{
			{Title: "My Issues", Filters: "is:open is:issue author:@me"},
			{Title: "Assigned", Filters: "is:open is:issue assignee:@me"},
		},
		Defaults: Defaults{PRsLimit: 20, IssuesLimit: 20},
	}
}

// Dir returns the directory holding config, credentials, and logs, honouring DirEnv.
func Dir() (string, error) {
	if override := os.Getenv(DirEnv); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, dirName), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// Load reads the config file, fills in defaults for anything absent, and
// validates the result. A missing file is not an error: it yields Default().
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config at %s: %w", path, err)
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	d := Default()
	if len(c.PRSections) == 0 {
		c.PRSections = d.PRSections
	}
	if len(c.IssueSections) == 0 {
		c.IssueSections = d.IssueSections
	}
	if c.Defaults.PRsLimit == 0 {
		c.Defaults.PRsLimit = d.Defaults.PRsLimit
	}
	if c.Defaults.IssuesLimit == 0 {
		c.Defaults.IssuesLimit = d.Defaults.IssuesLimit
	}
}

func (c *Config) validate() error {
	if err := validateSections("prSections", c.PRSections); err != nil {
		return err
	}
	if err := validateSections("issueSections", c.IssueSections); err != nil {
		return err
	}
	if err := validateLimit("prsLimit", c.Defaults.PRsLimit); err != nil {
		return err
	}
	return validateLimit("issuesLimit", c.Defaults.IssuesLimit)
}

func validateSections(field string, sections []Section) error {
	for i, s := range sections {
		if s.Title == "" {
			return fmt.Errorf("%s[%d]: title is required", field, i)
		}
		if s.Filters == "" {
			return fmt.Errorf("%s[%d] (%q): filters is required", field, i, s.Title)
		}
		for _, m := range sinceToken.FindAllStringSubmatch(s.Filters, -1) {
			if d, err := time.ParseDuration(m[1]); err != nil || d <= 0 {
				return fmt.Errorf("%s[%d] (%q): %q is not a length of time to look back, want something like 24h", field, i, s.Title, m[1])
			}
		}
	}
	return nil
}

func validateLimit(field string, limit int) error {
	if limit < 1 || limit > maxLimit {
		return fmt.Errorf("%s: %d is out of range, want 1 to %d", field, limit, maxLimit)
	}
	return nil
}
