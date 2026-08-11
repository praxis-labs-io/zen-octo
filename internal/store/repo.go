package store

import "github.com/zen-octo/zen-octo/internal/gh"

// Repo is the choices a picker draws from, held per repository. The zero value
// is one never fetched, which reads as idle and unloaded.
type Repo struct {
	Meta   gh.RepoMeta
	Status Status
	Err    error

	// Loaded marks metadata that has answered at least once, so a picker can
	// tell an empty repository from one still on its way.
	Loaded bool
}

// Repo is what a picker offers for a repository, keyed by "owner/name". Views
// read it, they never fetch it.
func (s Store) Repo(repo string) Repo { return s.repos[repo] }

// BeginRepoMeta marks a repository's metadata in flight and reports whether it
// started. It refuses one already on its way, so opening two pickers before the
// first answers costs one request rather than two.
//
// It also refuses one already loaded. Labels and branches change on the scale
// of days, and a reader who wants the newer set has the sync key.
func (s *Store) BeginRepoMeta(repo string) bool {
	held := s.repos[repo]
	if repo == "" || held.Status == StatusLoading || held.Loaded {
		return false
	}
	held.Status = StatusLoading
	s.putRepo(repo, held)
	return true
}

// RepoMetaApplied stores a repository's choices and folds the response into the
// budget.
func (s *Store) RepoMetaApplied(repo string, res gh.RepoMetaResult) {
	if repo == "" {
		return
	}
	s.putRepo(repo, Repo{Meta: res.Meta, Status: StatusReady, Loaded: true})
	s.adopt(res.RateLimit)
}

// RepoMetaFailed puts a repository into its error state, keeping whatever it
// already held, the same way a failed detail refetch does.
func (s *Store) RepoMetaFailed(repo string, err error) {
	if repo == "" {
		return
	}
	held := s.repos[repo]
	held.Status = StatusFailed
	held.Err = err
	s.putRepo(repo, held)
}

// InvalidateRepoMeta drops what a repository answered, so the next picker asks
// again. This is what makes the sync key reach metadata: without it BeginRepoMeta
// refuses every request after the first.
func (s *Store) InvalidateRepoMeta(repo string) { delete(s.repos, repo) }

// putRepo writes metadata, building the map if this Store was made without New.
// A nil map panics on write, and it would do it inside Update.
func (s *Store) putRepo(repo string, r Repo) {
	if s.repos == nil {
		s.repos = make(map[string]Repo)
	}
	s.repos[repo] = r
}
