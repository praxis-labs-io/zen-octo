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

// Branches is one branch search, held per repository. The zero value is one
// never run, which reads as idle and unloaded.
//
// It is not part of Repo, and that is the whole distinction between them. Repo
// is fetched once per repository and answers every picker that reads it; this
// is keyed by what somebody typed and is replaced on the next keystroke.
type Branches struct {
	// Query is the search these names answer. Two searches settle in whatever
	// order the network gives them, so a caller painting one has to be able to
	// tell whether it is still the one being asked.
	Query string

	// Default is what the repository calls its own default branch, which the
	// picker offers first. It rides along with every search rather than being
	// held apart: it costs one field on a call already being made, and holding
	// it separately would mean a second thing to load before the modal opens.
	Default string

	Names []string

	// More is what the search matched past the names returned.
	More int

	Status Status
	Err    error

	// Loaded marks a search that has answered at least once, so an empty result
	// can be told from one still on its way.
	Loaded bool
}

// Branches is the branch search held for a repository. Views read it, they
// never fetch it.
func (s Store) Branches(repo string) Branches { return s.branches[repo] }

// BeginBranches marks a branch search in flight and reports whether it started.
//
// It refuses a query already answered as well as one already on its way, so
// backspacing onto a search that has landed costs nothing and a repeated
// keystroke costs one request rather than two. Nothing is cached past the
// current query: the reader is typing, and every keystroke is a different
// question.
//
// A search that failed is not refused. It is the one state where asking the
// same question again is the reader retrying rather than the store forgetting
// what it holds, which is the same reading the Files tab takes of a diff whose
// last answer was an error.
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

// BranchesApplied stores a search's answer and folds the response into the
// budget.
//
// An answer to a query nobody is asking any more is dropped. Two searches out
// at once settle in the order the responses arrive, which is not the order they
// were typed, and taking the older one paints a list two keystrokes behind the
// filter above it.
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

// BranchesFailed puts a search into its error state, keeping whatever names it
// already held. A failed keystroke must not empty a list that was reading fine.
//
// It takes the query for the reason BranchesApplied does. A failure for a
// search two keystrokes ago would otherwise put an error over the one still
// running, and the reader would be told their search failed while it was on its
// way back.
func (s *Store) BranchesFailed(repo, query string, err error) {
	held := s.branches[repo]
	if repo == "" || held.Query != query {
		return
	}
	held.Status = StatusFailed
	held.Err = err
	s.putBranches(repo, held)
}

// InvalidateBranches drops the search a repository last answered, so the next
// picker asks again. This is what makes the sync key reach branches: without it
// BeginBranches refuses the opening search for the rest of the session, and a
// branch created in the browser never appears. On a repository small enough
// that comp.Picker draws no filter row there is no search to type either, so a
// restart is the only other way to reach it.
func (s *Store) InvalidateBranches(repo string) { delete(s.branches, repo) }

// putBranches writes a search, building the map if this Store was made without
// New, for the reason putRepo gives.
func (s *Store) putBranches(repo string, b Branches) {
	if s.branches == nil {
		s.branches = make(map[string]Branches)
	}
	s.branches[repo] = b
}
