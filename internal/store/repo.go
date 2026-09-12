package store

import "github.com/praxis-labs-io/zen-octo/internal/gh"

// Repo is the choices a picker draws from for one repository.
type Repo struct {
	Meta   gh.RepoMeta
	Status Status
	Err    error

	// Loaded is true once the metadata has answered.
	Loaded bool
}

// Repo is the metadata held for "owner/name", or the zero value where none was fetched.
func (s Store) Repo(repo string) Repo { return s.repos[repo] }

// BeginRepoMeta marks a repository's metadata in flight and reports whether it started.
// It refuses one in flight or already loaded.
func (s *Store) BeginRepoMeta(repo string) bool {
	held := s.repos[repo]
	if repo == "" || held.Status == StatusLoading || held.Loaded {
		return false
	}
	held.Status = StatusLoading
	s.putRepo(repo, held)
	return true
}

// RepoMetaApplied stores a repository's choices and folds the budget.
func (s *Store) RepoMetaApplied(repo string, res gh.RepoMetaResult) {
	if repo == "" {
		return
	}
	s.putRepo(repo, Repo{Meta: res.Meta, Status: StatusReady, Loaded: true})
	s.adopt(res.RateLimit)
}

// RepoMetaFailed puts a repository into its error state, keeping what it held.
func (s *Store) RepoMetaFailed(repo string, err error) {
	if repo == "" {
		return
	}
	held := s.repos[repo]
	held.Status = StatusFailed
	held.Err = err
	s.putRepo(repo, held)
}

// InvalidateRepoMeta drops a repository's metadata so the next BeginRepoMeta starts.
func (s *Store) InvalidateRepoMeta(repo string) { delete(s.repos, repo) }

func (s *Store) putRepo(repo string, r Repo) {
	if s.repos == nil {
		s.repos = make(map[string]Repo)
	}
	s.repos[repo] = r
}

// Branches is the latest branch search for one repository.
type Branches struct {
	// Query is the search these names answer.
	Query string

	// Default is the repository's default branch.
	Default string

	Names []string

	// More is how many matches the search left out.
	More int

	Status Status
	Err    error

	// Loaded is true once a search has answered.
	Loaded bool
}

// Branches is the branch search held for a repository.
func (s Store) Branches(repo string) Branches { return s.branches[repo] }

// BeginBranches marks a search in flight and reports whether it started. It refuses the held
// query while loading or answered, and retries one that failed.
func (s *Store) BeginBranches(repo, query string) bool {
	held := s.branches[repo]
	if repo == "" {
		return false
	}
	if held.Query == query && (held.Status == StatusLoading || held.Status == StatusReady) {
		return false
	}
	held.Query = query
	held.Status = StatusLoading
	s.putBranches(repo, held)
	return true
}

// BranchesApplied stores a search's answer and folds the budget. An answer to any query but the held one is dropped.
func (s *Store) BranchesApplied(repo string, res gh.BranchResult) {
	held := s.branches[repo]
	if repo == "" || held.Query != res.Query {
		return
	}

	s.putBranches(repo, Branches{
		Query:   res.Query,
		Default: res.Default,
		Names:   res.Branches,
		More:    res.More,
		Status:  StatusReady,
		Loaded:  true,
	})
	s.adopt(res.RateLimit)
}

// BranchesFailed puts the held search into its error state, keeping its names. A failure for another query is ignored.
func (s *Store) BranchesFailed(repo, query string, err error) {
	held := s.branches[repo]
	if repo == "" || held.Query != query {
		return
	}
	held.Status = StatusFailed
	held.Err = err
	s.putBranches(repo, held)
}

// InvalidateBranches drops the held search so the next BeginBranches starts.
func (s *Store) InvalidateBranches(repo string) { delete(s.branches, repo) }

func (s *Store) putBranches(repo string, b Branches) {
	if s.branches == nil {
		s.branches = make(map[string]Branches)
	}
	s.branches[repo] = b
}
