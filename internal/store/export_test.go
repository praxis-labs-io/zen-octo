package store

// The caps are constants rather than settings, so the tests outside this package
// fill past the real numbers rather than a smaller one injected for them.
const (
	DetailCap = detailCap
	FilesCap  = filesCap
	CommitCap = commitCap
)

// Cached reports how many details are held, which is the one thing eviction
// changes that no public reader can see: a dropped detail reads as never opened.
func (s Store) Cached() int { return s.details.len() }

// RowStamps is the same for the map syncRow writes. Its growth is the whole of
// what dropping a stamp buys, and restoreRows reads the same false without one.
func (s Store) RowStamps() int { return len(s.rowSeq) }
