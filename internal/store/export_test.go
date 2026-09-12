package store

const (
	DetailCap = detailCap
	FilesCap  = filesCap
	CommitCap = commitCap
)

func (s Store) Cached() int { return s.details.len() }

func (s Store) RowStamps() int { return len(s.rowSeq) }
