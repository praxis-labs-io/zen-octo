// Package config loads zen-octo's YAML configuration from ~/.zen-octo.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

// DirEnv overrides the config directory. Tests set it; users generally don't.
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

// Defaults holds settings that aren't tied to a single section.
type Defaults struct {
	PRsLimit    int `yaml:"prsLimit"`
	IssuesLimit int `yaml:"issuesLimit"`
}

// Config is the whole of what's on disk, after defaults are applied.
type Config struct {
	PRSections    []Section `yaml:"prSections"`
	IssueSections []Section `yaml:"issueSections"`
	Defaults      Defaults  `yaml:"defaults"`
	Theme         string    `yaml:"theme"`
}

// Default is what a user gets before they've written a config file.
func Default() *Config {
	return &Config{
		// GitHub's search API has one index for issues and pull requests, so
		// every filter needs is:pr or is:issue. Without it the limit gets spent
		// on the wrong kind and the section quietly undercounts.
		PRSections: []Section{
			{Title: "My PRs", Filters: "is:open is:pr author:@me"},
			{Title: "Needs My Review", Filters: "is:open is:pr review-requested:@me"},
			{Title: "Involved", Filters: "is:open is:pr involves:@me -author:@me"},
		},
		IssueSections: []Section{
			{Title: "My Issues", Filters: "is:open is:issue author:@me"},
			{Title: "Assigned", Filters: "is:open is:issue assignee:@me"},
		},
		Defaults: Defaults{PRsLimit: 20, IssuesLimit: 20},
		Theme:    "rose-pine-moon",
	}
}

// Dir returns the directory holding config, credentials, and logs.
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

// Path returns the full path to the config file.
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
	if c.Theme == "" {
		c.Theme = d.Theme
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
	}
	return nil
}

func validateLimit(field string, limit int) error {
	if limit < 1 || limit > maxLimit {
		return fmt.Errorf("%s: %d is out of range, want 1 to %d", field, limit, maxLimit)
	}
	return nil
}
