package app_test

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/config"
	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/tui/app"
	"github.com/praxis-labs-io/zen-octo/internal/tui/list"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

type fakeSearcher struct {
	rate   gh.RateLimit
	viewer gh.ViewerResult
	err    error

	viewerErr error

	mu              sync.Mutex
	prs             []gh.PullRequest
	queries         []string
	opens           []string
	pulses          []string
	diffs           []string
	commitDiffs     []string
	jobAsks         []int64
	jobLogAsks      []int64
	reruns          []int64
	runReruns       []runRerun
	details         map[string]gh.PullRequestDetail
	files           map[int][]gh.ChangedFile
	commitFiles     map[string][]gh.ChangedFile
	jobs            map[int64]gh.Job
	jobLogs         map[int64][]byte
	posted          []string
	replied         []string
	edited          []string
	deleted         []string
	bodies          []string
	settled         []string
	toggled         []string
	labelled        []string
	assigned        []string
	reviewed        []string
	moved           []string
	retargeted      []string
	merged          []gh.MergeOptions
	mergeState      gh.PRState
	deletedRefs     []string
	states          map[string]*gh.PRStateResult
	metaAsked       []string
	repoMetas       map[string]gh.RepoMeta
	metaErr         error
	branchQueries   []string
	branches        []string
	branchErr       error
	diffCounts      []int
	retargetedFiles int
	detailErr       error
	pulseErr        error
	filesErr        error
	commitErr       error
	jobErr          error
	jobLogErr       error
	rerunErr        error
	runRerunErr     error
	postErr         error
	requestErr      error

	deleteErr   error
	commitHold  time.Duration
	postHold    time.Duration
	gotLimit    int
	gotDeadline time.Time
	hadDeadline bool
}

func (f *fakeSearcher) Viewer(context.Context) (gh.ViewerResult, error) {
	if f.viewerErr != nil {
		return gh.ViewerResult{}, f.viewerErr
	}
	return f.viewer, nil
}

func (f *fakeSearcher) SearchPullRequests(ctx context.Context, query string, limit int) (gh.SearchResult, error) {
	f.mu.Lock()
	f.queries = append(f.queries, query)
	f.gotLimit = limit
	f.gotDeadline, f.hadDeadline = ctx.Deadline()
	prs := slices.Clone(f.prs)
	f.mu.Unlock()

	if f.err != nil {
		return gh.SearchResult{}, f.err
	}
	return gh.SearchResult{PullRequests: prs, RateLimit: f.rate}, nil
}

func (f *fakeSearcher) serve(prs []gh.PullRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prs = prs
}

func (f *fakeSearcher) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.queries)
}

func (f *fakeSearcher) calls() int { return len(f.asked()) }

func (f *fakeSearcher) PullRequest(_ context.Context, id, _ string) (gh.DetailResult, error) {
	f.mu.Lock()
	f.opens = append(f.opens, id)
	detail, err := f.details[id], f.detailErr
	for _, pr := range f.prs {
		if pr.ID == id {
			detail.PullRequest = pr
		}
	}
	f.mu.Unlock()

	if err != nil {
		return gh.DetailResult{}, err
	}
	return gh.DetailResult{Detail: detail, RateLimit: f.rate}, nil
}

func (f *fakeSearcher) Pulse(_ context.Context, id string) (gh.PulseResult, error) {
	f.mu.Lock()
	f.pulses = append(f.pulses, id)
	held, err := f.details[id], f.pulseErr
	f.mu.Unlock()

	if err != nil {
		return gh.PulseResult{}, err
	}
	return gh.PulseResult{
		Pulse: gh.Pulse{
			State:          held.State,
			IsDraft:        held.IsDraft,
			ReviewDecision: held.ReviewDecision,
			Merge:          held.Merge,
			Rollup:         held.Rollup,
			UpdatedAt:      held.UpdatedAt,
			HeadRefOid:     held.HeadRefOid,
		},
		RateLimit: f.rate,
	}, nil
}

func (f *fakeSearcher) pulsed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.pulses)
}

func (f *fakeSearcher) serveDetail(id, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.details == nil {
		f.details = make(map[string]gh.PullRequestDetail)
	}
	detail := gh.PullRequestDetail{
		Body:   body,
		Merge:  gh.MergeUnknown,
		Viewer: gh.ViewerActions{CanUpdate: true, CanClose: true, CanAssign: true},
	}
	for _, pr := range f.prs {
		if pr.ID == id {
			detail.PullRequest = pr
			detail.Rollup = gh.CheckRollup{State: pr.Checks}
		}
	}
	f.details[id] = detail
}

func (f *fakeSearcher) serveLabels(id string, labels []gh.Label) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.details == nil {
		f.details = make(map[string]gh.PullRequestDetail)
	}
	held := f.details[id]
	held.Labels = labels
	f.details[id] = held
}

func (f *fakeSearcher) serveAssignees(id string, assignees []gh.Actor) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.details == nil {
		f.details = make(map[string]gh.PullRequestDetail)
	}
	held := f.details[id]
	held.Assignees = assignees
	f.details[id] = held
}

func (f *fakeSearcher) serveReviewers(id string, reviewers []gh.Reviewer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.details == nil {
		f.details = make(map[string]gh.PullRequestDetail)
	}
	held := f.details[id]
	held.Reviewers = reviewers
	f.details[id] = held
}

func (f *fakeSearcher) serveCommits(id string, commits []gh.Commit) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.details == nil {
		f.details = make(map[string]gh.PullRequestDetail)
	}
	held := f.details[id]
	held.Commits = commits
	f.details[id] = held
}

func (f *fakeSearcher) failDetails(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.detailErr = err
}

func (f *fakeSearcher) opened() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.opens)
}

func (f *fakeSearcher) PullRequestFiles(_ context.Context, _, repo string, number, changed int) (gh.FilesResult, error) {
	f.mu.Lock()
	f.diffs = append(f.diffs, repo+"#"+strconv.Itoa(number))
	f.diffCounts = append(f.diffCounts, changed)
	files, err := f.files[number], f.filesErr
	f.mu.Unlock()

	if err != nil {
		return gh.FilesResult{}, err
	}
	return gh.FilesResult{Files: files, MoreFiles: max(0, changed-len(files))}, nil
}

func (f *fakeSearcher) SetFileViewed(_ context.Context, _, _ string, _ bool) error { return nil }

func (f *fakeSearcher) Job(_ context.Context, _ string, id int64) (gh.Job, error) {
	f.mu.Lock()
	f.jobAsks = append(f.jobAsks, id)
	job, err := f.jobs[id], f.jobErr
	f.mu.Unlock()
	if job.ID == 0 {
		job.ID = id
	}
	return job, err
}

func (f *fakeSearcher) JobLogs(_ context.Context, _ string, id int64) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobLogAsks = append(f.jobLogAsks, id)
	err := f.jobLogErr
	if err == nil {
		err = f.jobErr
	}
	return slices.Clone(f.jobLogs[id]), err
}

func (f *fakeSearcher) RerunJob(_ context.Context, _ string, id int64) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reruns = append(f.reruns, id)
	return time.Now(), f.rerunErr
}

func (f *fakeSearcher) RerunFailedJobs(_ context.Context, _ string, runID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runReruns = append(f.runReruns, runRerun{runID: runID})
	return f.runRerunErr
}

func (f *fakeSearcher) RerunAllJobs(_ context.Context, _ string, runID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runReruns = append(f.runReruns, runRerun{runID: runID, all: true})
	return f.runRerunErr
}

type runRerun struct {
	runID int64
	all   bool
}

func (f *fakeSearcher) servedJob(id int64, job gh.Job, log string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.jobs == nil {
		f.jobs = make(map[int64]gh.Job)
	}
	if f.jobLogs == nil {
		f.jobLogs = make(map[int64][]byte)
	}
	f.jobs[id], f.jobLogs[id] = job, []byte(log)
}

func (f *fakeSearcher) askedJobs() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.jobAsks)
}

func (f *fakeSearcher) askedJobLogs() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.jobLogAsks)
}

func (f *fakeSearcher) askedReruns() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.reruns)
}

func (f *fakeSearcher) serveFiles(number int, files []gh.ChangedFile) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.files == nil {
		f.files = make(map[int][]gh.ChangedFile)
	}
	f.files[number] = files
}

func (f *fakeSearcher) failFiles(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.filesErr = err
}

func (f *fakeSearcher) diffAsks() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.diffCounts)
}

func (f *fakeSearcher) fetched() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.diffs)
}

func (f *fakeSearcher) CommitFiles(_ context.Context, repo, sha string) (gh.FilesResult, error) {
	f.mu.Lock()
	f.commitDiffs = append(f.commitDiffs, repo+"@"+sha)
	files, err, hold := f.commitFiles[sha], f.commitErr, f.commitHold
	f.mu.Unlock()

	time.Sleep(hold)

	if err != nil {
		return gh.FilesResult{}, err
	}
	return gh.FilesResult{Files: files}, nil
}

func (f *fakeSearcher) AddComment(_ context.Context, subjectID, body string) (gh.CommentResult, error) {
	f.mu.Lock()
	f.posted = append(f.posted, subjectID+": "+body)
	err, hold := f.postErr, f.postHold
	f.mu.Unlock()

	time.Sleep(hold)

	if err != nil {
		return gh.CommentResult{}, err
	}
	return gh.CommentResult{
		Comment: gh.Comment{
			Kind: gh.CommentIssue, ID: "IC_POSTED",
			Author: gh.Actor{Login: "drucial"}, CreatedAt: time.Now(), Body: body,
			ViewerDidAuthor: true, CanEdit: true, CanDelete: true, CanReact: true,
		},
	}, nil
}

func (f *fakeSearcher) AddReply(_ context.Context, threadID, body string) (gh.CommentResult, error) {
	f.mu.Lock()
	f.replied = append(f.replied, threadID+": "+body)
	err, hold := f.postErr, f.postHold
	f.mu.Unlock()

	time.Sleep(hold)

	if err != nil {
		return gh.CommentResult{}, err
	}
	return gh.CommentResult{
		Comment: gh.Comment{
			Kind: gh.CommentThread, ID: "PRRC_POSTED",
			Author: gh.Actor{Login: "drucial"}, CreatedAt: time.Now(), Body: body,
			ViewerDidAuthor: true, CanEdit: true, CanDelete: true, CanReact: true,
		},
	}, nil
}

func (f *fakeSearcher) SetReaction(_ context.Context, subjectID string,
	content gh.ReactionContent, on bool,
) (gh.ReactionResult, error) {
	f.mu.Lock()
	f.toggled = append(f.toggled, subjectID+" "+string(content)+" "+strconv.FormatBool(on))
	err, hold := f.postErr, f.postHold
	f.mu.Unlock()

	time.Sleep(hold)

	if err != nil {
		return gh.ReactionResult{}, err
	}
	if !on {
		return gh.ReactionResult{}, nil
	}
	return gh.ReactionResult{Reactions: []gh.Reaction{
		{Content: content, Count: 9, Viewer: true},
	}}, nil
}

func (f *fakeSearcher) reactions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.toggled)
}

func (f *fakeSearcher) SetThreadResolved(_ context.Context, threadID string, resolved bool) (gh.ThreadResult, error) {
	f.mu.Lock()
	f.settled = append(f.settled, threadID+": "+strconv.FormatBool(resolved))
	err, hold := f.postErr, f.postHold
	f.mu.Unlock()

	time.Sleep(hold)

	if err != nil {
		return gh.ThreadResult{}, err
	}
	return gh.ThreadResult{
		ID: threadID, IsResolved: resolved, CanResolve: !resolved, CanUnresolve: resolved,
	}, nil
}

func (f *fakeSearcher) UpdateComment(_ context.Context, kind gh.CommentKind, id, body string) (gh.CommentResult, error) {
	f.mu.Lock()
	f.edited = append(f.edited, string(kind)+" "+id+": "+body)
	err, hold := f.postErr, f.postHold
	f.mu.Unlock()

	time.Sleep(hold)

	if err != nil {
		return gh.CommentResult{}, err
	}
	return gh.CommentResult{
		Comment: gh.Comment{
			Kind: kind, ID: id,
			Author: gh.Actor{Login: "drucial"}, CreatedAt: time.Now(), Body: body,
			ViewerDidAuthor: true, CanEdit: true, CanDelete: true, CanReact: true,
		},
	}, nil
}

func (f *fakeSearcher) DeleteComment(_ context.Context, kind gh.CommentKind, id string) error {
	f.mu.Lock()
	f.deleted = append(f.deleted, string(kind)+" "+id)
	err, hold := f.postErr, f.postHold
	f.mu.Unlock()

	time.Sleep(hold)
	return err
}

func (f *fakeSearcher) SetBody(_ context.Context, prID, body string) (gh.BodyResult, error) {
	f.mu.Lock()
	f.bodies = append(f.bodies, prID+": "+body)
	err, hold := f.postErr, f.postHold
	f.mu.Unlock()

	time.Sleep(hold)

	if err != nil {
		return gh.BodyResult{}, err
	}
	return gh.BodyResult{Body: body}, nil
}

func (f *fakeSearcher) edits() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.edited)
}

func (f *fakeSearcher) deletedComments() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.deleted)
}

func (f *fakeSearcher) describes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.bodies)
}

func (f *fakeSearcher) resolved() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.settled)
}

func (f *fakeSearcher) RepoMeta(_ context.Context, repo string) (gh.RepoMetaResult, error) {
	f.mu.Lock()
	f.metaAsked = append(f.metaAsked, repo)
	meta, err := f.repoMetas[repo], f.metaErr
	f.mu.Unlock()

	if err != nil {
		return gh.RepoMetaResult{}, err
	}
	return gh.RepoMetaResult{Meta: meta}, nil
}

func (f *fakeSearcher) serveRepoMeta(meta gh.RepoMeta) {
	f.serveRepoMetaFor("acme/rocket", meta)
}

func (f *fakeSearcher) serveRepoMetaFor(repo string, meta gh.RepoMeta) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.repoMetas == nil {
		f.repoMetas = make(map[string]gh.RepoMeta)
	}
	f.repoMetas[repo] = meta
}

func (f *fakeSearcher) metaCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.metaAsked)
}

func (f *fakeSearcher) SetLabels(_ context.Context, prID string, labelIDs []string) (gh.LabelsResult, error) {
	f.mu.Lock()
	f.labelled = append(f.labelled, prID+": "+strings.Join(labelIDs, ","))
	known, err, hold := f.repoMetas["acme/rocket"].Labels, f.postErr, f.postHold
	f.mu.Unlock()

	time.Sleep(hold)

	if err != nil {
		return gh.LabelsResult{}, err
	}

	out := make([]gh.Label, 0, len(labelIDs))
	for _, l := range known {
		if slices.Contains(labelIDs, l.ID) {
			out = append(out, l)
		}
	}
	return gh.LabelsResult{Labels: out}, nil
}

func (f *fakeSearcher) SetAssignees(_ context.Context, prID string, assigneeIDs []string) (gh.AssigneesResult, error) {
	f.mu.Lock()
	f.assigned = append(f.assigned, prID+": "+strings.Join(assigneeIDs, ","))
	known, err, hold := f.repoMetas["acme/rocket"].Users, f.postErr, f.postHold
	f.mu.Unlock()

	time.Sleep(hold)

	if err != nil {
		return gh.AssigneesResult{}, err
	}

	out := make([]gh.Actor, 0, len(assigneeIDs))
	for _, u := range known {
		if slices.Contains(assigneeIDs, u.ID) {
			out = append(out, u)
		}
	}

	f.mu.Lock()
	if held, ok := f.details[prID]; ok {
		held.Assignees = out
		f.details[prID] = held
	}
	f.mu.Unlock()

	return gh.AssigneesResult{Assignees: out}, nil
}

func (f *fakeSearcher) assigneeWrites() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.assigned)
}

func (f *fakeSearcher) RequestReviews(_ context.Context, repo string, number int, logins []string) error {
	f.mu.Lock()
	f.reviewed = append(f.reviewed, "+"+repo+"#"+strconv.Itoa(number)+": "+strings.Join(logins, ","))
	err, hold := cmp.Or(f.requestErr, f.postErr), f.postHold
	f.mu.Unlock()

	time.Sleep(hold)
	if err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.idOf(number)
	held := f.details[id]
	for _, l := range logins {
		if at := slices.IndexFunc(held.Reviewers, func(r gh.Reviewer) bool { return r.Actor.Login == l }); at >= 0 {
			held.Reviewers[at].Requested = true
			continue
		}
		held.Reviewers = append(held.Reviewers, gh.Reviewer{Actor: gh.Actor{Login: l}, Requested: true})
	}
	f.details[id] = held
	return nil
}

func (f *fakeSearcher) RemoveReviewRequests(_ context.Context, repo string, number int, logins []string) error {
	f.mu.Lock()
	f.reviewed = append(f.reviewed, "-"+repo+"#"+strconv.Itoa(number)+": "+strings.Join(logins, ","))
	err, hold := f.postErr, f.postHold
	f.mu.Unlock()

	time.Sleep(hold)
	if err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.idOf(number)
	held := f.details[id]
	panel := slices.Clone(held.Reviewers)
	for i := range panel {
		if panel[i].Requested && !panel[i].Team && slices.Contains(logins, panel[i].Actor.Login) {
			panel[i].Requested = false
		}
	}
	held.Reviewers = slices.DeleteFunc(panel, func(r gh.Reviewer) bool {
		return !r.Requested && r.State == "" && !r.Team
	})
	f.details[id] = held
	return nil
}

func TestTheFakeAnswersASearchWithRowsALaterWriteCannotEdit(t *testing.T) {
	f := &fakeSearcher{prs: samplePRs()}

	res, err := f.SearchPullRequests(context.Background(), "is:open", 20)
	if err != nil {
		t.Fatalf("SearchPullRequests() error = %v", err)
	}

	row := res.PullRequests[0]
	if _, err := f.SetBase(context.Background(), row.ID, "develop"); err != nil {
		t.Fatalf("SetBase() error = %v", err)
	}
	if got := res.PullRequests[0].BaseRefName; got != row.BaseRefName {
		t.Errorf("the row a search answered with became %q when a later write landed, want %q",
			got, row.BaseRefName)
	}
}

func TestTheFakeReadsItsRowsUnderItsOwnLock(t *testing.T) {
	f := &fakeSearcher{prs: samplePRs()}
	id := f.prs[0].ID

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := f.PullRequest(context.Background(), id, ""); err != nil {
				t.Errorf("PullRequest() error = %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := f.SetBase(context.Background(), id, "develop"); err != nil {
				t.Errorf("SetBase() error = %v", err)
			}
		}()
	}
	wg.Wait()
}

// Callers already hold the lock, and a Go mutex is not reentrant.
func (f *fakeSearcher) idOf(number int) string {
	for _, pr := range f.prs {
		if pr.Number == number {
			return pr.ID
		}
	}
	return ""
}

func (f *fakeSearcher) reviewerWrites() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.reviewed)
}

func (f *fakeSearcher) SetState(_ context.Context, prID string, to gh.PRTransition) (gh.PRStateResult, error) {
	f.mu.Lock()
	f.moved = append(f.moved, prID+": "+string(to))
	staged, err, hold := f.states[prID], f.postErr, f.postHold

	out := gh.PRStateResult{State: gh.PRStateOpen}
	for _, pr := range f.prs {
		if pr.ID == prID {
			out = gh.PRStateResult{State: pr.State, IsDraft: pr.IsDraft}
		}
	}
	f.mu.Unlock()

	time.Sleep(hold)

	if err != nil {
		return gh.PRStateResult{}, err
	}

	switch {
	case staged != nil:
		out = *staged
	case to == gh.TransitionReady:
		out.IsDraft = false
	case to == gh.TransitionDraft:
		out.IsDraft = true
	case to == gh.TransitionClose:
		out.State = gh.PRStateClosed
	case to == gh.TransitionReopen:
		out.State = gh.PRStateOpen
	}

	f.mu.Lock()
	for i := range f.prs {
		if f.prs[i].ID == prID {
			f.prs[i].State, f.prs[i].IsDraft = out.State, out.IsDraft
		}
	}
	f.mu.Unlock()

	return out, nil
}

func (f *fakeSearcher) serveState(id string, state gh.PRState, draft bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.states == nil {
		f.states = make(map[string]*gh.PRStateResult)
	}
	f.states[id] = &gh.PRStateResult{State: state, IsDraft: draft}
}

func (f *fakeSearcher) stateWrites() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.moved)
}

func (f *fakeSearcher) labelWrites() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.labelled)
}

func (f *fakeSearcher) answered() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.replied)
}

func (f *fakeSearcher) written() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.posted)
}

func (f *fakeSearcher) holdPosts() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.postHold = 50 * time.Millisecond
}

func (f *fakeSearcher) holdCommits() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commitHold = 50 * time.Millisecond
}

func (f *fakeSearcher) serveCommit(sha string, files []gh.ChangedFile) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.commitFiles == nil {
		f.commitFiles = make(map[string][]gh.ChangedFile)
	}
	f.commitFiles[sha] = files
}

func (f *fakeSearcher) failCommits(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commitErr = err
}

func (f *fakeSearcher) fetchedCommits() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.commitDiffs)
}

type querySearcher struct {
	results map[string]gh.SearchResult
	errs    map[string]error
}

func (f *querySearcher) Viewer(context.Context) (gh.ViewerResult, error) {
	return gh.ViewerResult{Viewer: gh.Actor{Login: "drucial"}}, nil
}

func (f *querySearcher) SearchPullRequests(_ context.Context, query string, _ int) (gh.SearchResult, error) {
	if err, ok := f.errs[query]; ok {
		return gh.SearchResult{}, err
	}
	return f.results[query], nil
}

func (f *querySearcher) PullRequest(_ context.Context, id, _ string) (gh.DetailResult, error) {
	return gh.DetailResult{Detail: gh.PullRequestDetail{PullRequest: gh.PullRequest{ID: id}}}, nil
}

func (f *querySearcher) Pulse(_ context.Context, _ string) (gh.PulseResult, error) {
	return gh.PulseResult{}, nil
}

func (f *querySearcher) PullRequestFiles(_ context.Context, _, _ string, _, _ int) (gh.FilesResult, error) {
	return gh.FilesResult{}, nil
}

func (f *querySearcher) SetFileViewed(_ context.Context, _, _ string, _ bool) error { return nil }

func (f *querySearcher) CommitFiles(_ context.Context, _, _ string) (gh.FilesResult, error) {
	return gh.FilesResult{}, nil
}

func (f *querySearcher) Job(_ context.Context, _ string, id int64) (gh.Job, error) {
	return gh.Job{ID: id}, nil
}

func (f *querySearcher) JobLogs(_ context.Context, _ string, _ int64) ([]byte, error) {
	return nil, nil
}

func (f *querySearcher) RerunJob(context.Context, string, int64) (time.Time, error) {
	return time.Now(), nil
}

func (f *querySearcher) RerunFailedJobs(context.Context, string, int64) error { return nil }
func (f *querySearcher) RerunAllJobs(context.Context, string, int64) error    { return nil }

func (f *querySearcher) AddComment(_ context.Context, _, _ string) (gh.CommentResult, error) {
	return gh.CommentResult{}, nil
}

func (f *querySearcher) AddReply(_ context.Context, _, _ string) (gh.CommentResult, error) {
	return gh.CommentResult{}, nil
}

func (f *querySearcher) SetThreadResolved(_ context.Context, _ string, _ bool) (gh.ThreadResult, error) {
	return gh.ThreadResult{}, nil
}

func (f *querySearcher) SetReaction(_ context.Context, _ string,
	_ gh.ReactionContent, _ bool,
) (gh.ReactionResult, error) {
	return gh.ReactionResult{}, nil
}

func (f *querySearcher) UpdateComment(_ context.Context, _ gh.CommentKind, _, _ string) (gh.CommentResult, error) {
	return gh.CommentResult{}, nil
}

func (f *querySearcher) DeleteComment(_ context.Context, _ gh.CommentKind, _ string) error {
	return nil
}

func (f *querySearcher) SetBody(_ context.Context, _, _ string) (gh.BodyResult, error) {
	return gh.BodyResult{}, nil
}

func (f *querySearcher) RepoMeta(_ context.Context, _ string) (gh.RepoMetaResult, error) {
	return gh.RepoMetaResult{}, nil
}

func (f *querySearcher) SetLabels(_ context.Context, _ string, _ []string) (gh.LabelsResult, error) {
	return gh.LabelsResult{}, nil
}

func (f *querySearcher) SetState(_ context.Context, _ string, _ gh.PRTransition) (gh.PRStateResult, error) {
	return gh.PRStateResult{}, nil
}

func (f *querySearcher) SetAssignees(_ context.Context, _ string, _ []string) (gh.AssigneesResult, error) {
	return gh.AssigneesResult{}, nil
}

func (f *querySearcher) SetBase(_ context.Context, _, _ string) (gh.BaseResult, error) {
	return gh.BaseResult{}, nil
}

func (f *querySearcher) Merge(_ context.Context, _ string, _ gh.MergeOptions) (gh.MergeResult, error) {
	return gh.MergeResult{}, nil
}

func (f *querySearcher) DeleteRef(_ context.Context, _ string) error { return nil }

func (f *querySearcher) Branches(_ context.Context, _, _ string) (gh.BranchResult, error) {
	return gh.BranchResult{}, nil
}

func (f *querySearcher) RequestReviews(_ context.Context, _ string, _ int, _ []string) error {
	return nil
}

func (f *querySearcher) RemoveReviewRequests(_ context.Context, _ string, _ int, _ []string) error {
	return nil
}

func testConfig() *config.Config {
	return &config.Config{
		PRSections: []config.Section{
			{Title: "My PRs", Filters: "is:open is:pr author:@me"},
			{Title: "Needs My Review", Filters: "is:open is:pr review-requested:@me"},
		},
		Defaults: config.Defaults{PRsLimit: 20, IssuesLimit: 20},
	}
}

func samplePRs() []gh.PullRequest {
	return []gh.PullRequest{
		{
			ID: "PR_412", Number: 412, Title: "Fix auth retry", Repository: "acme/rocket",
			URL:    "https://github.com/acme/rocket/pull/412",
			Author: gh.Actor{Login: "drucial"}, State: gh.PRStateOpen, BaseRefName: "main",
			HeadRefName: "fix-auth", Additions: 42, Deletions: 7, ChangedFiles: 3,
			Checks: gh.CheckStateSuccess, UpdatedAt: time.Now().Add(-2 * time.Hour),
		},
		{
			ID: "PR_408", Number: 408, Title: "Bump deps", Repository: "acme/rocket",
			Author: gh.Actor{Login: "drucial"}, State: gh.PRStateOpen, IsDraft: true,
			Checks: gh.CheckStateFailure, UpdatedAt: time.Now().Add(-30 * time.Hour),
		},
	}
}

func drive(t *testing.T, m tea.Model, msgs ...tea.Msg) tea.Model {
	t.Helper()

	m = settle(m, immediate(m.Init())...)
	return settle(m, msgs...)
}

func loaded(t *testing.T, client *fakeSearcher, width, height int) tea.Model {
	t.Helper()
	return drive(t, app.New(testConfig(), client, testSurface, nil), tea.WindowSizeMsg{Width: width, Height: height})
}

func settle(m tea.Model, msgs ...tea.Msg) tea.Model {
	queue := append([]tea.Msg(nil), msgs...)
	for range 64 {
		if len(queue) == 0 {
			break
		}

		var cmd tea.Cmd
		m, cmd = m.Update(queue[0])
		queue = append(queue[1:], immediate(cmd)...)
	}
	return m
}

// A command that does not answer at once is a timer, and is dropped so the suite never sleeps.
func immediate(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()

	select {
	case msg := <-done:
		if batch, ok := msg.(tea.BatchMsg); ok {
			var out []tea.Msg
			for _, sub := range batch {
				out = append(out, immediate(sub)...)
			}
			return out
		}
		if msg == nil {
			return nil
		}
		return []tea.Msg{msg}
	case <-time.After(20 * time.Millisecond):
		return nil
	}
}

func holdBack(m tea.Model, msg tea.Msg, want string) (tea.Model, []tea.Msg) {
	queue := []tea.Msg{msg}
	var held []tea.Msg

	for range 64 {
		if len(queue) == 0 {
			break
		}
		next := queue[0]
		queue = queue[1:]

		if strings.Contains(fmt.Sprintf("%T", next), want) {
			held = append(held, next)
			continue
		}

		var cmd tea.Cmd
		m, cmd = m.Update(next)
		queue = append(queue, immediate(cmd)...)
	}
	return m, held
}

func responses(cmd tea.Cmd) []tea.Msg {
	var out []tea.Msg
	for _, msg := range immediate(cmd) {
		if _, ok := msg.(spinner.TickMsg); !ok {
			out = append(out, msg)
		}
	}
	return out
}

func render(t *testing.T, m tea.Model) string {
	t.Helper()
	return m.View().Content
}

func press(m tea.Model, keys ...string) tea.Model {
	for _, k := range keys {
		m = settle(m, keyMsg(k))
	}
	return m
}

func settleOn(m tea.Model, sha string) tea.Model {
	return settle(m, prview.CommitSettleMsg{SHA: sha})
}

func settleJob(m tea.Model, check gh.Check, refresh bool) tea.Model {
	return settle(m, prview.JobSettleMsg{
		Key: check.Key(), JobID: check.JobID, Refresh: refresh,
	})
}

func settleSearch(m tea.Model, queries ...string) tea.Model {
	for _, q := range queries {
		m = settle(m, prview.BranchSettleMsg{Query: q})
	}
	return m
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "ctrl+enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	default:
		return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
	}
}

func write(m tea.Model, text string) tea.Model {
	for _, r := range text {
		m = settle(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func TestRendersFetchedPullRequests(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)

	out := render(t, m)
	for _, want := range []string{"My PRs", "#412", "Fix auth retry", "acme/rocket", "drucial", "#408"} {
		if !strings.Contains(out, want) {
			t.Errorf("view is missing %q\n%s", want, out)
		}
	}
}

func TestEverySectionFetchesOnceWithItsOwnFilters(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	drive(t, app.New(testConfig(), client, testSurface, nil))

	want := []string{"is:open is:pr author:@me", "is:open is:pr review-requested:@me"}
	got := client.asked()
	slices.Sort(got)
	slices.Sort(want)

	if !slices.Equal(got, want) {
		t.Errorf("queries = %q, want each section's filters exactly once and unmodified", got)
	}
	if client.gotLimit != 20 {
		t.Errorf("limit = %d, want 20", client.gotLimit)
	}
}

func TestEveryTabCarriesItsOwnCount(t *testing.T) {
	client := &querySearcher{results: map[string]gh.SearchResult{
		"is:open is:pr author:@me":           {PullRequests: manyPRs(5)},
		"is:open is:pr review-requested:@me": {PullRequests: manyPRs(2)},
	}}

	top := strings.Split(stripANSI(render(t, drive(t, app.New(testConfig(), client, testSurface, nil), tea.WindowSizeMsg{Width: 160, Height: 40}))), "\n")[0]

	for _, want := range []string{"My PRs (5)", "Needs My Review (2)"} {
		if !strings.Contains(top, want) {
			t.Errorf("tab strip = %q, want %q in it", top, want)
		}
	}
}

func TestRendersEmptySectionWithoutClaimingAnError(t *testing.T) {
	out := render(t, loaded(t, &fakeSearcher{prs: nil}, 120, 40))

	if !strings.Contains(out, "Nothing matches this section.") {
		t.Errorf("view = %q, want the empty-section message", out)
	}
	if strings.Contains(out, "Failed to load") {
		t.Error("view claims a failure for an empty result")
	}
}

func TestRendersTheFixCommandWhenAScopeIsMissing(t *testing.T) {
	client := &fakeSearcher{err: errors.New("HTTP 403\nYour gh token is missing the workflow scope. Run:\n  gh auth refresh -s workflow")}
	out := render(t, loaded(t, client, 120, 40))

	if !strings.Contains(out, "Failed to load") {
		t.Errorf("view = %q, want the failure label", out)
	}
	if !strings.Contains(out, "gh auth refresh -s workflow") {
		t.Errorf("view = %q, want the fix command carried through to the screen", out)
	}
}

func TestCursorMovesAndStopsAtTheEnds(t *testing.T) {
	base := loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)

	moved := press(base, "j", "j")
	if !strings.Contains(selectedText(t, moved), "#408") {
		t.Errorf("selection = %q, want it clamped to the last row", selectedText(t, moved))
	}

	back := press(moved, "k", "k", "k")
	if !strings.Contains(selectedText(t, back), "#412") {
		t.Errorf("selection = %q, want it clamped to the first row", selectedText(t, back))
	}
}

func TestRefreshRefetchesEverySection(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 120, 40)

	if client.calls() != 2 {
		t.Fatalf("calls = %d after load, want one per section", client.calls())
	}

	settle(m, keyMsg("s"))
	if client.calls() != 4 {
		t.Errorf("calls = %d after refresh, want another per section", client.calls())
	}
}

func TestQuitKeys(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{name: "q", key: tea.KeyPressMsg{Code: 'q', Text: "q"}},
		{name: "ctrl+c", key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := drive(t, app.New(testConfig(), &fakeSearcher{prs: samplePRs()}, testSurface, nil))

			_, cmd := m.Update(tt.key)
			if cmd == nil {
				t.Fatal("produced no command, want quit")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Error("did not produce a QuitMsg")
			}
		})
	}
}

func TestSelectionPaintsEveryCellNotJustTheFirst(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)

	line := selectedLine(t, m)
	if line == "" {
		t.Fatal("no row carries the selection background")
	}

	if got := strings.Count(line, selectionSeq()); got < 7 {
		t.Errorf("selection background appears %d times, want one per cell (>=7)\n%q", got, line)
	}
}

func TestRefreshClearsTheStaleError(t *testing.T) {
	client := &fakeSearcher{err: errors.New("boom, the first attempt failed")}
	m := loaded(t, client, 120, 40)

	if !strings.Contains(render(t, m), "Failed to load") {
		t.Fatal("setup: expected the first fetch to render a failure")
	}

	next, _ := m.Update(list.RefreshMsg{})
	out := render(t, next)
	if strings.Contains(out, "boom, the first attempt failed") {
		t.Errorf("view still shows the previous error during the retry\n%s", out)
	}
	if !strings.Contains(out, "Loading pull requests") {
		t.Errorf("view = %q, want the loading state during the retry", out)
	}
}

func TestRefreshKeepsTheCursorOnTheSamePullRequest(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := press(loaded(t, client, 120, 40), "j")

	client.serve(append([]gh.PullRequest{{
		ID: "PR_NEW", Number: 500, Title: "Brand new", Repository: "acme/rocket",
		State: gh.PRStateOpen, UpdatedAt: time.Now(),
	}}, samplePRs()...))
	m = settle(m, keyMsg("s"))

	if got := selectedText(t, m); !strings.Contains(got, "#408") {
		t.Errorf("selection = %q, want it still on #408 after the refresh", got)
	}
}

func TestRefreshClampsTheCursorWhenTheRowIsGone(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := press(loaded(t, client, 120, 40), "j")

	client.serve(samplePRs()[:1])
	m = settle(m, keyMsg("s"))

	if got := selectedText(t, m); !strings.Contains(got, "#412") {
		t.Errorf("selection = %q, want it clamped onto the remaining row", got)
	}
}

func TestTheFrameNeverExceedsTheTerminal(t *testing.T) {
	sizes := []struct{ width, height int }{
		{width: 200, height: 60},
		{width: 120, height: 40},
		{width: 90, height: 24},
		{width: app.MinWidth + 1, height: app.MinHeight + 1},
		{width: app.MinWidth, height: app.MinHeight},
		{width: 40, height: 5},
		{width: 20, height: 2},
	}

	for _, size := range sizes {
		name := fmt.Sprintf("%dx%d", size.width, size.height)
		t.Run(name, func(t *testing.T) {
			m := loaded(t, &fakeSearcher{prs: manyPRs(60)}, size.width, size.height)

			for _, stage := range []struct {
				what string
				m    tea.Model
			}{
				{what: "list", m: m},
				{what: "detail", m: press(m, "enter")},
				{what: "help", m: press(m, "?")},
			} {
				out := render(t, stage.m)
				lines := strings.Split(out, "\n")
				if len(lines) > size.height {
					t.Errorf("%s frame is %d lines, want no more than %d", stage.what, len(lines), size.height)
				}
				for i, line := range lines {
					if w := lipgloss.Width(line); w > size.width {
						t.Errorf("%s frame line %d is %d cells wide, want no more than %d", stage.what, i, w, size.width)
					}
				}
			}
		})
	}
}

func TestCursorStaysVisibleWhenScrollingPastTheFold(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: manyPRs(60)}, 120, 24)

	for range 30 {
		m = press(m, "j")
	}

	if selectedText(t, m) == "" {
		t.Fatal("the selected row is not in the rendered frame after scrolling")
	}
	if got := selectedText(t, m); !strings.Contains(got, "#30") {
		t.Errorf("selection = %q, want the 31st row (#30)", got)
	}
}

func TestEnterOpensTheDetailAndEscapeComesBack(t *testing.T) {
	m := press(loaded(t, &fakeSearcher{prs: samplePRs()}, 160, 40), "j")

	detail := press(m, "enter")
	out := render(t, detail)
	if !strings.Contains(stripANSI(out), "Conversation") {
		t.Errorf("detail = %q, want the conversation tab strip", out)
	}
	if !strings.Contains(stripANSI(out), "#408 Bump deps") {
		t.Errorf("detail = %q, want the selected pull request", out)
	}

	back := press(detail, "esc")
	if !strings.Contains(render(t, back), "Fix auth retry") {
		t.Error("escape did not return to the list")
	}
	if got := selectedText(t, back); !strings.Contains(got, "#408") {
		t.Errorf("selection = %q, want the same row still selected", got)
	}
}

func TestTheHintLinePairsOpposedKeysUnderOneVerb(t *testing.T) {
	tests := []struct {
		name  string
		frame func(*testing.T) string
		want  []string
	}{
		{
			name: "list",
			frame: func(t *testing.T) string {
				return lastLine(render(t, loaded(t, &fakeSearcher{prs: samplePRs()}, 160, 40)))
			},
			want: []string{"j/k move", "⏎ open", "[/] tab", "? help"},
		},
		{
			name: "detail",
			frame: func(t *testing.T) string {
				return lastLine(render(t, press(loaded(t, &fakeSearcher{prs: samplePRs()}, 160, 40), "enter")))
			},
			want: []string{"j/k move", "[/] tab", "d details", "esc back"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bar := stripANSI(tt.frame(t))
			for _, want := range tt.want {
				if !strings.Contains(bar, want) {
					t.Errorf("status bar = %q, want %q in it", strings.TrimSpace(bar), want)
				}
			}
		})
	}
}

func TestTheRailCollapsesOnANarrowTerminal(t *testing.T) {
	const railOnly = "Author"

	wide := press(loaded(t, &fakeSearcher{prs: samplePRs()}, 160, 40), "enter")
	if !strings.Contains(render(t, wide), railOnly) {
		t.Error("the rail is missing on a wide terminal")
	}

	narrow := press(loaded(t, &fakeSearcher{prs: samplePRs()}, 100, 40), "enter")
	if strings.Contains(render(t, narrow), railOnly) {
		t.Error("the rail is still on screen at 100 columns, want it collapsed")
	}

	if !strings.Contains(render(t, press(narrow, "d")), railOnly) {
		t.Error("the toggle did not bring the rail back on a narrow terminal")
	}
	if strings.Contains(render(t, press(wide, "d")), railOnly) {
		t.Error("the toggle did not hide the rail on a wide terminal")
	}
}

func TestHelpOverlaysTheScreenAndDismisses(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)

	open := press(m, "?")
	out := render(t, open)
	if !strings.Contains(out, "Keys") {
		t.Errorf("help = %q, want the overlay title", out)
	}
	if !strings.Contains(out, "half page down") {
		t.Errorf("help = %q, want a binding's declared description", out)
	}

	if strings.Contains(render(t, press(open, "?")), "half page down") {
		t.Error("pressing ? again did not dismiss the help")
	}
}

func TestHelpReflowsRatherThanLosingItsBorder(t *testing.T) {
	for _, width := range []int{160, 100, 80, 60} {
		t.Run(fmt.Sprintf("%d", width), func(t *testing.T) {
			out := render(t, press(loaded(t, &fakeSearcher{prs: samplePRs()}, width, 24), "?"))

			var top string
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, "Keys") {
					top = line
					break
				}
			}
			if top == "" {
				t.Fatal("the help overlay is not on screen")
			}
			if !strings.Contains(top, "╮") {
				t.Errorf("the modal lost its top-right corner, so it was sheared rather than reflowed\n%s", top)
			}
			if lipgloss.Width(top) != width {
				t.Errorf("the composited line is %d cells, want %d", lipgloss.Width(top), width)
			}
		})
	}
}

func TestHelpSwallowsScreenKeys(t *testing.T) {
	m := press(loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40), "?")

	if _, cmd := m.Update(keyMsg("enter")); cmd != nil {
		t.Error("enter reached the list while help was up")
	}
	if !strings.Contains(render(t, press(m, "esc")), "Fix auth retry") {
		t.Error("escape did not dismiss the help")
	}
}

func TestTabDoesNotChangeSectionOnTheList(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 120, 40)

	before := render(t, m)
	if got := render(t, settle(m, keyMsg("tab"))); got != before {
		t.Error("tab moved the list, which is ] and [ on both screens")
	}
	if got := render(t, settle(m, keyMsg("]"))); got == before {
		t.Error("] did not change section")
	}
}

func TestChangingSectionRefetchesNothing(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 120, 40)
	before := client.calls()

	m = settle(m, keyMsg("]"))

	if got := client.calls(); got != before {
		t.Errorf("calls went from %d to %d, want the switch to fetch nothing", before, got)
	}
	if !strings.Contains(render(t, m), "Fix auth retry") {
		t.Error("the second section's rows are not on screen")
	}
}

func TestTheStatusBarCarriesTheRateLimit(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), rate: gh.RateLimit{Limit: 5000, Cost: 1, Remaining: 421}}

	if out := render(t, loaded(t, client, 120, 40)); !strings.Contains(out, "421") {
		t.Errorf("view = %q, want the remaining budget in the status bar", out)
	}
}

func TestRefreshAnnouncesItselfOnceButTheFirstLoadDoesNot(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 120, 40)

	if strings.Contains(render(t, m), "Refreshed") {
		t.Error("the first load raised a toast, want the rows to speak for themselves")
	}

	out := render(t, settle(m, keyMsg("s")))
	if !strings.Contains(out, "Refreshed 2 sections") {
		t.Errorf("view = %q, want the refresh to report what came back", out)
	}
}

func TestAToastLandsOnTheRightAndLeavesTheHintsAlone(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)

	bar := stripANSI(lastLine(render(t, settle(m, keyMsg("s")))))

	toast := strings.Index(bar, "Refreshed")
	hints := strings.Index(bar, "j/k move")
	if toast < 0 || hints < 0 {
		t.Fatalf("status bar = %q, want both the toast and the hints on it", strings.TrimSpace(bar))
	}
	if toast < hints {
		t.Errorf("status bar = %q, want the toast to the right of the hints", strings.TrimSpace(bar))
	}
}

func TestTheDetailHintsNameOnlyWhatTheTabCanDo(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	tests := []struct {
		name       string
		to         []string
		want, gone []string
	}{
		{
			name: "conversation",
			want: []string{"⏎ open", "h/l panes", "d details"},
			gone: []string{"{/} block", "space expand"},
		},
		{
			name: "conversation, on the page",
			to:   []string{"l"},
			want: []string{"{/} block", "space expand", "d details"},
			gone: []string{"h/l panes"},
		},
		{
			name: "commits",
			to:   []string{"]"},
			want: []string{"{/} block"},
			gone: []string{"space expand", "d details"},
		},
		{
			name: "checks",
			to:   []string{"]", "]"},
			gone: []string{"{/} block", "space expand", "d details"},
		},
		{
			name: "files",
			to:   []string{"]", "]", "]"},
			want: []string{"{/} block", "space expand"},
			gone: []string{"d details"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := press(loaded(t, client, 160, 40), append([]string{"enter"}, tt.to...)...)
			bar := stripANSI(lastLine(render(t, m)))

			for _, want := range append(tt.want, "j/k move", "[/] tab") {
				if !strings.Contains(bar, want) {
					t.Errorf("status bar = %q, want %q on it", strings.TrimSpace(bar), want)
				}
			}
			for _, gone := range tt.gone {
				if strings.Contains(bar, gone) {
					t.Errorf("status bar = %q, want %q off it: the key is inert here", strings.TrimSpace(bar), gone)
				}
			}
		})
	}
}

func TestTheBarGoesQuietWhileAModalHoldsTheKeyboard(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveRepoMeta(gh.RepoMeta{Labels: []gh.Label{{ID: "L_bug", Name: "bug"}}})

	m := press(loaded(t, client, 160, 40), "enter", "1", "j", "j", "j", "enter")
	if out := stripANSI(render(t, m)); !strings.Contains(out, "Add label") {
		t.Fatalf("setup: the label picker did not open:\n%s", out)
	}

	if bar := stripANSI(lastLine(render(t, m))); strings.Contains(bar, "d details") {
		t.Errorf("status bar = %q, want the screen's hints off it while a picker is up", strings.TrimSpace(bar))
	}
}

func TestTheListBarNamesNeitherTheSectionNorAHealthyBudget(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), rate: gh.RateLimit{Limit: 5000, Cost: 1, Remaining: 4821}}

	bar := stripANSI(lastLine(render(t, loaded(t, client, 120, 40))))

	if strings.Contains(bar, "My PRs") {
		t.Errorf("status bar = %q, want the section named only by the tab", strings.TrimSpace(bar))
	}
	if strings.Contains(bar, "4821") {
		t.Errorf("status bar = %q, want a healthy budget left off", strings.TrimSpace(bar))
	}
}

func TestTheRefreshToastWaitsForTheLastSection(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)

	next, cmd := m.Update(list.RefreshMsg{})
	landed := responses(cmd)
	if len(landed) < 2 {
		t.Fatalf("setup: the refresh produced %d responses, want one per section", len(landed))
	}

	next = settle(next, landed[0])
	if strings.Contains(render(t, next), "Refreshed") {
		t.Error("the toast fired while a section was still in flight")
	}

	if out := render(t, settle(next, landed[1:]...)); !strings.Contains(out, "Refreshed 2 sections") {
		t.Errorf("view = %q, want the toast once the last section landed", out)
	}
}

func TestTheRefreshWaitsOnASectionAlreadyInFlight(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	var m tea.Model = app.New(testConfig(), client, testSurface, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	initial := responses(m.Init())
	if len(initial) != 3 {
		t.Fatalf("setup: startup produced %d responses, want the viewer and one per section", len(initial))
	}
	sections := initial[1:]

	m, _ = m.Update(sections[0])
	m, cmd := m.Update(list.RefreshMsg{})

	m = settle(m, responses(cmd)...)
	if out := stripANSI(render(t, m)); strings.Contains(out, "Refreshed") {
		t.Error("the toast fired while the section the refresh adopted was still out")
	}

	if out := stripANSI(render(t, settle(m, sections[1]))); !strings.Contains(out, "Refreshed 2 sections") {
		t.Errorf("view = %q, want the toast to count the section it waited on", out)
	}
}

func TestARefreshThatFailsSaysSo(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	m := loaded(t, client, 120, 40)

	client.err = errors.New("context deadline exceeded")
	out := render(t, settle(m, keyMsg("s")))

	if !strings.Contains(out, "Refresh failed") {
		t.Errorf("view = %q, want the toast to report the failure", out)
	}
}

func TestFetchCarriesADeadline(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	drive(t, app.New(testConfig(), client, testSurface, nil))

	if !client.hadDeadline {
		t.Fatal("the fetch context has no deadline, so a hung request spins forever")
	}
	if until := time.Until(client.gotDeadline); until <= 0 || until > time.Minute {
		t.Errorf("deadline is %v away, want a sane positive bound", until)
	}
}

func TestALeftoverThemeNameSaysSoRatherThanBeingDropped(t *testing.T) {
	cfg := testConfig()
	cfg.Theme = config.Theme{Named: "rose-pine-moon"}

	m := drive(t, app.New(cfg, &fakeSearcher{prs: samplePRs()}, testSurface, nil), tea.WindowSizeMsg{Width: 160, Height: 40})

	out := render(t, m)
	if !strings.Contains(out, "rose-pine-moon") {
		t.Errorf("view = %q, want it to name the theme setting it ignored", out)
	}
	if !strings.Contains(out, "accent") {
		t.Errorf("view = %q, want it to name what to write instead", out)
	}
}

func TestOverridesShowNoNotice(t *testing.T) {
	cfg := testConfig()
	cfg.Theme = config.Theme{Colors: map[string]string{"accent": "#ff0000"}}

	m := drive(t, app.New(cfg, &fakeSearcher{prs: samplePRs()}, testSurface, nil), tea.WindowSizeMsg{Width: 160, Height: 40})
	if strings.Contains(render(t, m), "Theme names are gone") {
		t.Error("a config carrying color overrides produced a notice")
	}
}

func TestAnOverrideReachesTheScreen(t *testing.T) {
	cfg := testConfig()
	cfg.Theme = config.Theme{Colors: map[string]string{"accent": "#ff0000"}}

	m := drive(t, app.New(cfg, &fakeSearcher{prs: samplePRs()}, testSurface, nil), tea.WindowSizeMsg{Width: 160, Height: 40})

	want := fgSeq(lipgloss.Color("#ff0000"))
	if !strings.Contains(render(t, m), want) {
		t.Errorf("view does not carry %q, want the overridden accent painted", want)
	}
}

func TestTheRootPaintsTheThemesBackground(t *testing.T) {
	m := drive(t, app.New(testConfig(), &fakeSearcher{prs: samplePRs()}, testSurface, nil), tea.WindowSizeMsg{Width: 160, Height: 40})

	got := m.View().BackgroundColor
	if got == nil {
		t.Fatal("View().BackgroundColor is nil, want the background the shades were derived against")
	}
	if r, g, b, _ := got.RGBA(); r>>8 != 0x23 || g>>8 != 0x21 || b>>8 != 0x36 {
		t.Errorf("BackgroundColor = %d,%d,%d, want the reported 23,21,36", r>>8, g>>8, b>>8)
	}
}

func TestTransparentPaintsNoBackground(t *testing.T) {
	cfg := testConfig()
	cfg.Transparent = true

	m := drive(t, app.New(cfg, &fakeSearcher{prs: samplePRs()}, testSurface, nil), tea.WindowSizeMsg{Width: 160, Height: 40})

	if got := m.View().BackgroundColor; got != nil {
		t.Errorf("View().BackgroundColor = %v, want nil under transparent", got)
	}
}

func TestANamedBackgroundReachesThePaint(t *testing.T) {
	cfg := testConfig()
	cfg.Theme = config.Theme{Colors: map[string]string{"background": "#faf4ed"}}

	m := drive(t, app.New(cfg, &fakeSearcher{prs: samplePRs()}, testSurface, nil), tea.WindowSizeMsg{Width: 160, Height: 40})

	got := m.View().BackgroundColor
	if got == nil {
		t.Fatal("View().BackgroundColor is nil, want the named background")
	}
	if r, g, b, _ := got.RGBA(); r>>8 != 0xfa || g>>8 != 0xf4 || b>>8 != 0xed {
		t.Errorf("BackgroundColor = %d,%d,%d, want the named fa,f4,ed", r>>8, g>>8, b>>8)
	}
}

func TestScrollingFollowsTheCursorARowAtATime(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: manyPRs(60)}, 120, 24)

	prev := topRow(t, m)
	for i := range 30 {
		m = press(m, "j")

		top := topRow(t, m)
		if top < prev || top > prev+1 {
			t.Fatalf("press %d moved the window from row %d to row %d, want at most one row", i, prev, top)
		}
		prev = top
	}

	if prev == 0 {
		t.Error("the window never moved, so nothing here was tested")
	}
}

func topRow(t *testing.T, m tea.Model) int {
	t.Helper()

	for _, l := range strings.Split(stripANSI(render(t, m)), "\n") {
		i := strings.Index(l, "Change ")
		if i < 0 {
			continue
		}
		n, err := strconv.Atoi(strings.Fields(l[i+len("Change "):])[0])
		if err != nil {
			t.Fatalf("cannot read a row number out of %q: %v", l, err)
		}
		return n
	}
	t.Fatal("no pull request on screen")
	return 0
}

func TestShrinkingTheTerminalKeepsTheSelectionOnScreen(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: manyPRs(60)}, 120, 40)
	for range 30 {
		m = press(m, "j")
	}

	m = settle(m, tea.WindowSizeMsg{Width: 120, Height: 24})

	if got := selectedText(t, m); !strings.Contains(got, "#30") {
		t.Errorf("selection = %q, want row 30 still on screen after the shrink", got)
	}
}

func TestPageKeysMoveTheCursor(t *testing.T) {
	tests := []struct {
		name string
		keys []tea.KeyPressMsg
		want string
	}{
		{name: "page down", keys: []tea.KeyPressMsg{ctrl('f')}, want: "#7"},
		{name: "page down then back up", keys: []tea.KeyPressMsg{ctrl('f'), ctrl('b')}, want: "#0"},
		{name: "half page down", keys: []tea.KeyPressMsg{ctrl('d')}, want: "#3"},
		{name: "half page down twice, half back", keys: []tea.KeyPressMsg{ctrl('d'), ctrl('d'), ctrl('u')}, want: "#3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := loaded(t, &fakeSearcher{prs: manyPRs(60)}, 120, 24)
			for _, k := range tt.keys {
				m = settle(m, k)
			}

			if got := selectedText(t, m); !strings.Contains(got, tt.want+" ") {
				t.Errorf("selection = %q, want %s", got, tt.want)
			}
		})
	}
}

func TestAFailedSectionIsTheOnlyOneShowingAnError(t *testing.T) {
	client := &querySearcher{
		errs:    map[string]error{"is:open is:pr author:@me": errors.New("context deadline exceeded")},
		results: map[string]gh.SearchResult{"is:open is:pr review-requested:@me": {PullRequests: samplePRs()}},
	}

	m := drive(t, app.New(testConfig(), client, testSurface, nil), tea.WindowSizeMsg{Width: 120, Height: 40})

	first := render(t, m)
	if !strings.Contains(first, "context deadline exceeded") {
		t.Fatalf("the failed section does not show its own error\n%s", first)
	}

	second := render(t, settle(m, keyMsg("]")))
	if strings.Contains(second, "Failed to load") {
		t.Errorf("the failure followed the user to a section that loaded fine\n%s", second)
	}
	if !strings.Contains(second, "Fix auth retry") {
		t.Error("the loaded rows are not on screen")
	}
}

func TestTheStatusBarCarriesTheLowestBudgetSeen(t *testing.T) {
	window := time.Now().Add(time.Hour)
	client := &querySearcher{results: map[string]gh.SearchResult{
		"is:open is:pr author:@me": {
			PullRequests: samplePRs(),
			RateLimit:    gh.RateLimit{Limit: 5000, Remaining: 419, ResetAt: window},
		},
		"is:open is:pr review-requested:@me": {
			RateLimit: gh.RateLimit{Limit: 5000, Remaining: 420, ResetAt: window},
		},
	}}

	out := render(t, drive(t, app.New(testConfig(), client, testSurface, nil), tea.WindowSizeMsg{Width: 120, Height: 40}))
	if !strings.Contains(out, "419") {
		t.Errorf("view = %q, want the lowest remaining across the responses", out)
	}
}

func TestTheBarCarriesWhoOpenedItWhenNothingElseNeedsTheSide(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")
	if got := lastLine(render(t, m)); !strings.Contains(got, "@drucial") {
		t.Errorf("status bar = %q, want who opened the pull request on it", strings.TrimSpace(got))
	}

	if got := lastLine(render(t, loaded(t, client, 160, 40))); strings.Contains(got, "@drucial") {
		t.Errorf("list bar = %q, want the readout off a screen with no pull request", strings.TrimSpace(got))
	}
}

func TestALowBudgetOutranksTheReadout(t *testing.T) {
	window := time.Now().Add(time.Hour)
	client := &querySearcher{results: map[string]gh.SearchResult{
		"is:open is:pr author:@me": {
			PullRequests: samplePRs(),
			RateLimit:    gh.RateLimit{Limit: 5000, Remaining: 419, ResetAt: window},
		},
	}}

	m := press(drive(t, app.New(testConfig(), client, testSurface, nil), tea.WindowSizeMsg{Width: 160, Height: 40}), "enter")

	got := lastLine(render(t, m))
	if !strings.Contains(got, "419") {
		t.Errorf("status bar = %q, want the budget while it is low", strings.TrimSpace(got))
	}
	if strings.Contains(got, "@drucial") {
		t.Errorf("status bar = %q, want the readout to give way to the budget", strings.TrimSpace(got))
	}
}

func TestTheSpinnerKeepsTickingBehindTheDetailScreen(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)

	m, _ = m.Update(list.RefreshMsg{})

	var open tea.Cmd
	m, open = m.Update(keyMsg("enter"))
	m = settle(m, immediate(open)...)

	if _, cmd := m.Update(spinner.TickMsg{}); cmd == nil {
		t.Error("the tick produced no follow-up, so the spinner freezes mid-fetch")
	}
}

func TestHidingTheRailSticksAcrossPullRequests(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 160, 40)

	hidden := press(m, "enter", "d")
	if strings.Contains(render(t, hidden), "Branch") {
		t.Fatal("setup: d did not hide the rail")
	}

	if strings.Contains(render(t, press(hidden, "esc", "enter")), "Branch") {
		t.Error("reopening the pull request brought the rail back")
	}
}

func TestTheConfigNoticeReadsAsAWarning(t *testing.T) {
	cfg := testConfig()
	cfg.Theme = config.Theme{Named: "rose-pine-moon"}

	m := drive(t, app.New(cfg, &fakeSearcher{prs: samplePRs()}, testSurface, nil), tea.WindowSizeMsg{Width: 160, Height: 40})

	for _, line := range strings.Split(render(t, m), "\n") {
		if !strings.Contains(line, "Theme names are gone") {
			continue
		}
		if !strings.Contains(line, fgSeq(testTheme.Warning)) {
			t.Error("the notice renders in the same grey as the key hints")
		}
		return
	}
	t.Fatal("the notice is not on screen")
}

func TestAnUnknownSyntaxThemeIsReported(t *testing.T) {
	cfg := testConfig()
	cfg.SyntaxTheme = "not-a-chroma-style"

	m := drive(t, app.New(cfg, &fakeSearcher{prs: samplePRs()}, testSurface, nil), tea.WindowSizeMsg{Width: 200, Height: 40})

	if !strings.Contains(stripANSI(render(t, m)), `Unknown syntax theme "not-a-chroma-style"`) {
		t.Error("an unknown syntax theme falls back with nothing said")
	}
}

func TestAThemesOwnSyntaxStyleRaisesNoNotice(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 200, 40)

	if strings.Contains(stripANSI(render(t, m)), "Unknown syntax theme") {
		t.Error("the default theme names a style Chroma does not ship")
	}
}

func TestTheViewerIsAskedForAtStartup(t *testing.T) {
	client := &fakeSearcher{
		err: errors.New("every section is down"),
		viewer: gh.ViewerResult{
			Viewer:    gh.Actor{Login: "drucial"},
			RateLimit: gh.RateLimit{Limit: 5000, Cost: 1, Remaining: 499},
		},
	}

	if out := render(t, loaded(t, client, 120, 40)); !strings.Contains(out, "499") {
		t.Errorf("view = %q, want the budget the viewer response carried", out)
	}
}

func TestAViewerThatCannotBeReadChangesNothingOnScreen(t *testing.T) {
	rate := gh.RateLimit{Limit: 5000, Cost: 1, Remaining: 4820}

	broken := &fakeSearcher{prs: samplePRs(), rate: rate, viewerErr: errors.New("boom")}
	fine := &fakeSearcher{
		prs: samplePRs(), rate: rate,
		viewer: gh.ViewerResult{Viewer: gh.Actor{Login: "drucial"}},
	}

	if got, want := render(t, loaded(t, broken, 120, 40)), render(t, loaded(t, fine, 120, 40)); got != want {
		t.Errorf("a failed viewer lookup changed the frame:\n%q\nwant\n%q", got, want)
	}
}

func TestTheBudgetShowsAtZeroAndNotBeforeItIsKnown(t *testing.T) {
	spent := &fakeSearcher{prs: samplePRs(), rate: gh.RateLimit{Limit: 5000, Cost: 1, Remaining: 0}}
	if out := render(t, loaded(t, spent, 120, 40)); !strings.Contains(out, "◆ 0") {
		t.Errorf("view = %q, want the budget still readable once it is gone", out)
	}

	failed := &fakeSearcher{err: errors.New("boom")}
	if strings.Contains(render(t, loaded(t, failed, 120, 40)), "◆") {
		t.Error("the status bar shows a budget it has never been told")
	}
}

func ctrl(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }

func fgSeq(c color.Color) string { return sgrParams(lipgloss.NewStyle().Foreground(c)) }

// One repo and one clock reading, so the newest-first tiebreak cannot reorder rows.
func manyPRs(n int) []gh.PullRequest {
	at := time.Now()

	prs := make([]gh.PullRequest, n)
	for i := range prs {
		prs[i] = gh.PullRequest{
			ID: fmt.Sprintf("PR_%d", i), Number: i, Title: fmt.Sprintf("Change %d", i),
			Repository: "acme/rocket", State: gh.PRStateOpen, UpdatedAt: at,
		}
	}
	return prs
}

func selectionSeq() string {
	r, g, b, _ := testTheme.SelectedBackground.RGBA()
	return fmt.Sprintf("48;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

func selectedLine(t *testing.T, m tea.Model) string {
	t.Helper()

	var out []string
	for _, line := range strings.Split(render(t, m), "\n") {
		if strings.Contains(line, selectionSeq()) {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func selectedText(t *testing.T, m tea.Model) string {
	t.Helper()
	return stripANSI(selectedLine(t, m))
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func opening(m tea.Model) (tea.Model, tea.Cmd) {
	m, cmd := m.Update(keyMsg("enter"))
	for _, msg := range immediate(cmd) {
		m, cmd = m.Update(msg)
	}
	return m, cmd
}

func TestOpeningAPullRequestFetchesItOnce(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")

	if got := client.opened(); len(got) != 1 || got[0] != "PR_412" {
		t.Errorf("opened %v, want one fetch for PR_412", got)
	}
	if !strings.Contains(stripANSI(render(t, m)), "Caps the backoff at 30s.") {
		t.Error("the conversation never reached the screen")
	}
}

func TestReopeningPaintsFromWhatIsHeldAndRefetchesBehindIt(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")
	if !strings.Contains(stripANSI(render(t, m)), "Caps the backoff at 30s.") {
		t.Fatal("setup: the first open never loaded")
	}

	again, pending := opening(press(m, "esc"))
	if !strings.Contains(stripANSI(render(t, again)), "Caps the backoff at 30s.") {
		t.Error("the second open waited on the network rather than painting what was held")
	}

	settle(again, immediate(pending)...)
	if got := client.opened(); len(got) != 2 {
		t.Errorf("opened %v, want the reopen to have refetched", got)
	}
}

func TestAResponseForAPullRequestYouLeftDoesNotReachTheScreen(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "the auth retry description")
	client.serveDetail("PR_408", "the dependency bump description")

	first, stale := opening(loaded(t, client, 160, 40))
	elsewhere := press(press(first, "esc"), "j", "enter")

	if !strings.Contains(stripANSI(render(t, elsewhere)), "the dependency bump description") {
		t.Fatal("setup: the second pull request never loaded")
	}

	settled := settle(elsewhere, immediate(stale)...)
	out := stripANSI(render(t, settled))
	if strings.Contains(out, "the auth retry description") {
		t.Error("a response from the pull request that was left landed on the one on screen")
	}
	if !strings.Contains(out, "the dependency bump description") {
		t.Error("the screen lost what it was showing")
	}
}

func TestAFailedRefetchKeepsTheConversationAndSaysSo(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")
	client.failDetails(errors.New("no such host"))

	again := press(press(m, "esc"), "enter")
	out := stripANSI(render(t, again))

	if !strings.Contains(out, "Caps the backoff at 30s.") {
		t.Error("the failed refetch emptied a screen that was reading fine")
	}
	if !strings.Contains(out, "Could not refresh #412") {
		t.Errorf("frame = %q, want a toast naming the pull request that failed", out)
	}
}

func TestAFirstOpenSaysItIsLoading(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	waiting, _ := opening(loaded(t, client, 160, 40))
	out := stripANSI(render(t, waiting))

	if !strings.Contains(out, "Loading the conversation") {
		t.Error("the first open renders nothing while it waits")
	}
	if !strings.ContainsAny(out, "⣾⣽⣻⢿⡿⣟⣯⣷") {
		t.Errorf("frame = %q, want a spinner beside the label", out)
	}
}

func TestOpeningMovesTheBudget(t *testing.T) {
	client := &fakeSearcher{
		prs:  samplePRs(),
		rate: gh.RateLimit{Limit: 5000, Remaining: 400, ResetAt: time.Now().Add(time.Hour)},
	}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := loaded(t, client, 160, 40)
	client.rate = gh.RateLimit{Limit: 5000, Remaining: 397, ResetAt: client.rate.ResetAt}

	if out := render(t, press(m, "enter")); !strings.Contains(out, "397") {
		t.Errorf("frame = %q, want the budget the detail response carried", out)
	}
}

func TestOpeningArmsTheDetailSpinner(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	waiting, pending := opening(loaded(t, client, 160, 40))

	var started spinner.TickMsg
	var armed bool
	for _, msg := range immediate(pending) {
		if got, ok := msg.(spinner.TickMsg); ok {
			started, armed = got, true
		}
	}
	if !armed {
		t.Fatal("opening armed no spinner, so the glyph would never move")
	}

	before := stripANSI(render(t, waiting))
	moved, _ := waiting.Update(started)
	if stripANSI(render(t, moved)) == before {
		t.Error("the tick did not reach the detail screen")
	}
}

func TestReopeningWhileTheFetchIsStillOutKeepsTheSpinnerRunning(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	opened, pending := opening(loaded(t, client, 160, 40))
	back := settle(opened, keyMsg("esc"))

	again, reopened := opening(back)

	var tick spinner.TickMsg
	var armed bool
	for _, msg := range immediate(reopened) {
		if got, ok := msg.(spinner.TickMsg); ok {
			tick, armed = got, true
		}
	}
	if !armed {
		t.Fatal("the reopen armed no spinner, so the glyph would sit frozen")
	}

	before := stripANSI(render(t, again))
	moved, _ := again.Update(tick)
	if stripANSI(render(t, moved)) == before {
		t.Error("the tick did not reach the reopened screen")
	}

	settle(again, immediate(pending)...)
	if got := client.opened(); len(got) != 1 {
		t.Errorf("opened %v, want the one request that was already in flight", got)
	}
}

func TestTheDiffIsNotFetchedUntilTheFilesTabIsOpened(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveFiles(412, sampleFiles())

	m := press(loaded(t, client, 160, 40), "enter")
	if got := client.fetched(); len(got) != 0 {
		t.Fatalf("fetched %v before the tab was opened", got)
	}

	m = press(m, "]", "]", "]")
	if got := client.fetched(); len(got) != 1 || got[0] != "acme/rocket#412" {
		t.Errorf("fetched %v, want one diff for acme/rocket#412", got)
	}
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the diff never reached the screen")
	}
}

func TestTabbingBackToFilesDoesNotFetchAgain(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveFiles(412, sampleFiles())

	m := press(loaded(t, client, 160, 40), "enter")
	m = press(m, "]", "]", "]")
	m = press(m, "]")
	press(m, "]", "]", "]")

	if got := client.fetched(); len(got) != 1 {
		t.Errorf("fetched %v, want one request", got)
	}
}

func TestReopeningAPullRequestRefetchesItsDiff(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveFiles(412, sampleFiles())

	m := press(loaded(t, client, 160, 40), "enter", "]", "]", "]")
	if got := client.fetched(); len(got) != 1 {
		t.Fatalf("setup: fetched %v, want one request", got)
	}

	m = press(m, "esc", "enter", "]", "]", "]")

	if got := client.fetched(); len(got) != 2 {
		t.Errorf("fetched %v, want the diff asked for again on the reopen", got)
	}
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the diff is not on screen after the reopen")
	}
}

func TestOnlyTheCommitTheCursorStopsOnIsFetched(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{
		{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"},
		{SHA: "7b20ef4a11", Short: "7b20ef4", Headline: "Drop the count"},
	})
	client.serveCommit("7b20ef4a11", sampleFiles())

	m := press(loaded(t, client, 160, 40), "enter", "]", "1", "j")
	if got := client.fetchedCommits(); len(got) != 0 {
		t.Fatalf("fetched %v before the cursor stopped anywhere", got)
	}

	m = settleOn(m, "a3f91c2d5e")
	if got := client.fetchedCommits(); len(got) != 0 {
		t.Fatalf("fetched %v for a commit the cursor walked past", got)
	}

	m = settleOn(m, "7b20ef4a11")
	want := "acme/rocket@7b20ef4a11"
	if got := client.fetchedCommits(); len(got) != 1 || got[0] != want {
		t.Errorf("fetched %v, want one request for %q", got, want)
	}
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the commit's diff never reached the screen")
	}

	if settleOn(m, "7b20ef4a11"); len(client.fetchedCommits()) != 1 {
		t.Errorf("fetched %v, want the second settle to cost nothing",
			client.fetchedCommits())
	}
}

func TestTheCommitsTabFetchesWhatItOpensOn(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{
		{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"},
	})
	client.serveCommit("a3f91c2d5e", sampleFiles())

	m := settleOn(press(loaded(t, client, 160, 40), "enter", "]"), "a3f91c2d5e")

	if got := client.fetchedCommits(); len(got) != 1 {
		t.Fatalf("fetched %v, want the commit the tab opened on", got)
	}
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the tab opened without its diff")
	}
}

func TestACommitAlreadyReadIsNotFetchedAgain(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{
		{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"},
		{SHA: "7b20ef4a11", Short: "7b20ef4", Headline: "Drop the count"},
	})
	client.serveCommit("a3f91c2d5e", sampleFiles())
	client.serveCommit("7b20ef4a11", sampleFiles())

	m := settleOn(press(loaded(t, client, 160, 40), "enter", "]", "1"), "a3f91c2d5e")
	m = settleOn(press(m, "j"), "7b20ef4a11")
	m = settleOn(press(m, "k"), "a3f91c2d5e")

	want := []string{"acme/rocket@a3f91c2d5e", "acme/rocket@7b20ef4a11"}
	if got := client.fetchedCommits(); !slices.Equal(got, want) {
		t.Errorf("fetched %v, want %v: the second read of a commit is cached", got, want)
	}
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the cached diff never reached the screen")
	}
}

func TestWalkingBackToACachedCommitNeverSpins(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{
		{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"},
		{SHA: "7b20ef4a11", Short: "7b20ef4", Headline: "Drop the count"},
	})
	client.serveCommit("a3f91c2d5e", sampleFiles())
	client.serveCommit("7b20ef4a11", sampleFiles())

	m := settleOn(press(loaded(t, client, 160, 40), "enter", "]", "1"), "a3f91c2d5e")
	m = settleOn(press(m, "j"), "7b20ef4a11")

	m = press(m, "k")
	m, _ = m.Update(prview.CommitSettleMsg{SHA: "a3f91c2d5e"})

	if strings.Contains(stripANSI(render(t, m)), "Loading the diff") {
		t.Error("the pane spun on the way to a diff the store already held")
	}
	if got := client.fetchedCommits(); len(got) != 2 {
		t.Errorf("fetched %v, want the cached commit answered without a request", got)
	}
}

func TestReturningToACommitStillInFlightKeepsTheSpinnerAlive(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{
		{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"},
		{SHA: "7b20ef4a11", Short: "7b20ef4", Headline: "Drop the count"},
	})
	client.holdCommits()

	m := settleOn(press(loaded(t, client, 160, 40), "enter", "]", "1"), "a3f91c2d5e")
	m = settleOn(press(m, "j"), "7b20ef4a11")
	m = settleOn(press(m, "k"), "a3f91c2d5e")

	if !strings.Contains(stripANSI(render(t, m)), "Loading the diff") {
		t.Fatal("the pane dropped out of its loading state with the request still out")
	}
	if got := client.fetchedCommits(); len(got) != 2 {
		t.Errorf("fetched %v, want the return to ride the request already out", got)
	}

	before := spinnerGlyph(render(t, m), "Loading the diff")
	m, _ = m.Update(spinner.TickMsg{})
	after := spinnerGlyph(render(t, m), "Loading the diff")

	if before == "" || after == "" {
		t.Fatalf("no spinner on the pane, glyphs %q and %q", before, after)
	}
	if before == after {
		t.Error("the spinner froze with the request still out")
	}
}

func spinnerGlyph(frame, label string) string {
	for _, line := range strings.Split(stripANSI(frame), "\n") {
		at := strings.Index(line, label)
		if at <= 0 {
			continue
		}
		if lead := []rune(strings.TrimRight(line[:at], " ")); len(lead) > 0 {
			return string(lead[len(lead)-1])
		}
	}
	return ""
}

func TestAFailedCommitDiffSaysSoOnTheTab(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"}})
	client.failCommits(errors.New("context deadline exceeded"))

	m := settleOn(press(loaded(t, client, 160, 40), "enter", "]"), "a3f91c2d5e")

	if !strings.Contains(stripANSI(render(t, m)), "Could not load the diff") {
		t.Error("a failed commit diff reads as an empty one")
	}
}

func TestWalkingBackOntoAFailedCommitRetriesIt(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{
		{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"},
		{SHA: "7b20ef4a11", Short: "7b20ef4", Headline: "Drop the count"},
	})
	client.failCommits(errors.New("context deadline exceeded"))

	m := settleOn(press(loaded(t, client, 160, 40), "enter", "]", "1"), "a3f91c2d5e")
	if got := client.fetchedCommits(); len(got) != 1 {
		t.Fatalf("fetched %v, want the first request", got)
	}

	client.failCommits(nil)
	client.serveCommit("a3f91c2d5e", sampleFiles())
	m = settleOn(press(m, "j"), "7b20ef4a11")
	m = settleOn(press(m, "k"), "a3f91c2d5e")

	if got := client.fetchedCommits(); len(got) != 3 {
		t.Errorf("fetched %v, want the failed commit asked for again", got)
	}
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the retry never reached the screen")
	}
}

func refreshing(m tea.Model) (tea.Model, tea.Cmd) {
	m, cmd := m.Update(keyMsg("s"))
	for _, msg := range immediate(cmd) {
		m, cmd = m.Update(msg)
	}
	return m, cmd
}

func TestRefreshingTheDetailRefetchesTheConversation(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")
	client.serveDetail("PR_412", "Caps the backoff at 60s.")
	m = press(m, "s")

	if got := client.opened(); len(got) != 2 {
		t.Errorf("opened %v, want the refresh to have refetched", got)
	}
	if !strings.Contains(stripANSI(render(t, m)), "Caps the backoff at 60s.") {
		t.Error("the refreshed conversation never reached the screen")
	}
}

func TestRefreshingTheConversationAsksForNoDiff(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"}})

	m := press(loaded(t, client, 160, 40), "enter", "s")

	if got := client.fetched(); len(got) != 0 {
		t.Errorf("fetched %v, want no diff for a refresh on the conversation", got)
	}
	if got := client.fetchedCommits(); len(got) != 0 {
		t.Errorf("fetched commits %v, want none", got)
	}

	press(m, "]", "]", "s")
	if got := client.fetched(); len(got) != 0 {
		t.Errorf("fetched %v, want no diff for a refresh on the checks", got)
	}
}

func TestRefreshingOnTheFilesTabRefetchesTheDiff(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveFiles(412, sampleFiles())

	m := press(loaded(t, client, 160, 40), "enter", "]", "]", "]")
	if got := client.fetched(); len(got) != 1 {
		t.Fatalf("setup: fetched %v, want one request", got)
	}

	m = press(m, "s")

	if got := client.fetched(); len(got) != 2 {
		t.Errorf("fetched %v, want the diff asked for again", got)
	}
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the diff left the screen over the refresh")
	}
}

func TestRefreshingOnTheCommitsTabRefetchesTheCommitOnThePane(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveCommits("PR_412", []gh.Commit{{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"}})
	client.serveCommit("a3f91c2d5e", sampleFiles())

	m := settleOn(press(loaded(t, client, 160, 40), "enter", "]"), "a3f91c2d5e")
	if got := client.fetchedCommits(); len(got) != 1 {
		t.Fatalf("setup: fetched %v, want one request", got)
	}

	m = press(m, "s")

	want := []string{"acme/rocket@a3f91c2d5e", "acme/rocket@a3f91c2d5e"}
	if got := client.fetchedCommits(); !slices.Equal(got, want) {
		t.Errorf("fetched commits %v, want %v", got, want)
	}
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the commit diff left the screen over the refresh")
	}
}

func TestARefreshKeepsTheConversationOnScreen(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")
	waiting, pending := refreshing(m)

	out := stripANSI(render(t, waiting))
	if !strings.Contains(out, "Caps the backoff at 30s.") {
		t.Error("the conversation went away while the refresh was out")
	}
	if strings.Contains(out, "Loading the conversation") {
		t.Error("the screen spun over content it was already showing")
	}
	settle(waiting, immediate(pending)...)
}

func TestARefreshSpinsInTheStatusBar(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")
	waiting, pending := refreshing(m)

	if !strings.Contains(lastLine(render(t, waiting)), "Refreshing") {
		t.Errorf("status bar = %q, want the refresh on it", strings.TrimSpace(lastLine(render(t, waiting))))
	}

	done := settle(waiting, immediate(pending)...)
	if strings.Contains(lastLine(render(t, done)), "Refreshing") {
		t.Error("the bar is still spinning after the refresh landed")
	}
}

func TestASyncOnTheListSpinsInTheStatusBar(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	waiting, pending := refreshing(loaded(t, client, 160, 40))

	if !strings.Contains(lastLine(render(t, waiting)), "Refreshing") {
		t.Errorf("status bar = %q, want the sync on it", strings.TrimSpace(lastLine(render(t, waiting))))
	}

	done := settle(waiting, immediate(pending)...)
	if strings.Contains(lastLine(render(t, done)), "Refreshing") {
		t.Error("the bar is still spinning after the sync landed")
	}
}

func TestAFirstLoadDoesNotSpinInTheStatusBar(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m, _ := app.New(testConfig(), client, testSurface, nil).Update(tea.WindowSizeMsg{Width: 160, Height: 40})

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "Loading pull requests") {
		t.Fatalf("setup: the first load never spun over the pane:\n%s", out)
	}
	if strings.Contains(lastLine(render(t, m)), "Refreshing") {
		t.Error("the bar spun over a first load, which the pane is already reporting")
	}
}

func TestTheDetailRefreshToastNamesThePullRequest(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter", "s")

	if !strings.Contains(lastLine(render(t, m)), "Refreshed #412") {
		t.Errorf("status bar = %q, want the refresh reported", strings.TrimSpace(lastLine(render(t, m))))
	}
}

func TestTheDetailRefreshToastWaitsForTheDiff(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveFiles(412, sampleFiles())

	m := press(loaded(t, client, 160, 40), "enter", "]", "]", "]")
	waiting, pending := refreshing(m)

	answers := responses(pending)
	if len(answers) != 2 {
		t.Fatalf("the refresh started %d requests, want the detail and the diff", len(answers))
	}

	half := settle(waiting, answers[0])
	if strings.Contains(lastLine(render(t, half)), "Refreshed") {
		t.Error("the refresh reported itself with a request still out")
	}

	done := settle(half, answers[1])
	if !strings.Contains(lastLine(render(t, done)), "Refreshed #412") {
		t.Errorf("status bar = %q, want the refresh reported once both landed",
			strings.TrimSpace(lastLine(render(t, done))))
	}
}

func TestADetailRefreshReportsItselfOnce(t *testing.T) {
	boom := errors.New("context deadline exceeded")

	tests := []struct {
		name string
		fail func(*fakeSearcher)
		want string
	}{
		{"both back", func(*fakeSearcher) {}, "Refreshed #412"},
		{"both failed", func(f *fakeSearcher) { f.failDetails(boom); f.failFiles(boom) }, "Refresh failed"},
		{"diff failed", func(f *fakeSearcher) { f.failFiles(boom) }, "Refreshed #412, the diff failed"},
		{"detail failed", func(f *fakeSearcher) { f.failDetails(boom) }, "Refreshed the diff, #412 failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeSearcher{prs: samplePRs()}
			client.serveDetail("PR_412", "Caps the backoff at 30s.")
			client.serveFiles(412, sampleFiles())

			m := press(loaded(t, client, 160, 40), "enter", "]", "]", "]")
			tt.fail(client)
			m = press(m, "s")

			bar := lastLine(render(t, m))
			if !strings.Contains(bar, tt.want) {
				t.Errorf("status bar = %q, want %q", strings.TrimSpace(bar), tt.want)
			}
			if strings.Contains(bar, "Could not refresh") {
				t.Errorf("status bar = %q, want the summary rather than a per-request failure",
					strings.TrimSpace(bar))
			}
		})
	}
}

func TestARefreshWithNoDiffOutNeverBlamesTheDiff(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")
	client.failDetails(errors.New("context deadline exceeded"))
	m = press(m, "s")

	bar := lastLine(render(t, m))
	if strings.Contains(bar, "diff") {
		t.Errorf("status bar = %q, want no diff named by a refresh that asked for none",
			strings.TrimSpace(bar))
	}
	if !strings.Contains(bar, "Refresh failed") {
		t.Errorf("status bar = %q, want the failure reported", strings.TrimSpace(bar))
	}
}

func TestASecondRefreshJoinsTheOneStillRunning(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveCommits("PR_412", []gh.Commit{{SHA: "a3f91c2d5e", Short: "a3f91c2", Headline: "Cap the backoff"}})
	client.serveCommit("a3f91c2d5e", sampleFiles())

	m := settleOn(press(loaded(t, client, 160, 40), "enter", "]"), "a3f91c2d5e")
	m = press(m, "[")

	client.failDetails(errors.New("context deadline exceeded"))
	waiting, pending := refreshing(m)
	again, alsoPending := refreshing(press(waiting, "]"))

	done := settle(settle(again, immediate(alsoPending)...), immediate(pending)...)

	bar := lastLine(render(t, done))
	if !strings.Contains(bar, "Refreshed the diff, #412 failed") {
		t.Errorf("status bar = %q, want both legs reported together", strings.TrimSpace(bar))
	}
	if strings.Contains(bar, "Could not refresh") {
		t.Errorf("status bar = %q, want the summary rather than a per-request failure",
			strings.TrimSpace(bar))
	}
}

func TestRefreshingTwiceWhileTheFirstIsOutCostsOneRequest(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")
	waiting, pending := refreshing(m)
	again, _ := refreshing(waiting)

	settle(again, immediate(pending)...)
	if got := client.opened(); len(got) != 2 {
		t.Errorf("opened %v, want the open and one refresh", got)
	}
}

func TestRefreshingRetriesADiffThatFailed(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.failFiles(errors.New("context deadline exceeded"))

	m := press(loaded(t, client, 160, 40), "enter", "]", "]", "]")
	if !strings.Contains(stripANSI(render(t, m)), "Could not load the diff") {
		t.Fatal("setup: the diff did not fail")
	}

	client.failFiles(nil)
	client.serveFiles(412, sampleFiles())
	m = press(m, "s")

	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the retried diff never reached the pane")
	}
}

func TestLeavingTheDetailStopsTheRefreshSpinner(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(loaded(t, client, 160, 40), "enter")
	waiting, _ := refreshing(m)

	if got := lastLine(render(t, press(waiting, "esc"))); strings.Contains(got, "Refreshing") {
		t.Errorf("the list's status bar = %q, want no refresh left running on it", strings.TrimSpace(got))
	}
}

func TestTheDetailStatusBarCarriesNothingButItsHints(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	for _, width := range []int{100, 120, 160, 200} {
		bar := stripANSI(lastLine(render(t, press(loaded(t, client, width, 40), "enter"))))
		for _, unwanted := range []string{"#412", "acme/rocket"} {
			if strings.Contains(bar, unwanted) {
				t.Errorf("width %d: the status bar still carries %q: %q", width, unwanted, strings.TrimSpace(bar))
			}
		}
		if !strings.Contains(bar, "j/k move") {
			t.Errorf("width %d: the status bar has no hints on it: %q", width, strings.TrimSpace(bar))
		}
	}
}

func TestAFailedDiffFetchSaysSoOnTheTab(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.failFiles(errors.New("context deadline exceeded"))

	m := press(loaded(t, client, 160, 40), "enter")
	m = press(m, "]", "]", "]")

	if !strings.Contains(stripANSI(render(t, m)), "Could not load the diff") {
		t.Error("a failed diff fetch reads as an empty one")
	}
}

func TestADiffDoesNotFollowTheReaderToTheNextPullRequest(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveFiles(412, sampleFiles())

	m := press(loaded(t, client, 160, 40), "enter", "]", "]", "]")
	if !strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Fatal("the first diff never landed")
	}

	m = press(m, "esc", "j", "enter", "]", "]", "]")

	if strings.Contains(stripANSI(render(t, m)), "delay = min(delay*2, fetchTimeout)") {
		t.Error("the first pull request's diff is showing on the second")
	}
	if got := client.fetched(); len(got) != 2 || got[1] != "acme/rocket#408" {
		t.Errorf("fetched %v, want a second request for #408", got)
	}
}

func sampleFiles() []gh.ChangedFile {
	return []gh.ChangedFile{{
		Path: "internal/gh/client.go", Status: gh.FileModified, Additions: 2, Deletions: 1,
		Hunks: []gh.Hunk{{
			Header: "@@ -40,4 +40,5 @@",
			Lines: []gh.DiffLine{
				{Kind: gh.DiffContext, Old: 40, New: 40, Content: "\tfor {"},
				{Kind: gh.DiffRemoved, Old: 41, Content: "\t\ttime.Sleep(delay)"},
				{Kind: gh.DiffAdded, New: 41, Content: "\t\tdelay = min(delay*2, fetchTimeout)"},
			},
		}},
	}}
}

func lastLine(frame string) string {
	lines := strings.Split(stripANSI(frame), "\n")
	return lines[len(lines)-1]
}

func composed(t *testing.T, client *fakeSearcher, body string) tea.Model {
	t.Helper()

	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	m := press(loaded(t, client, 160, 40), "enter", "c")
	return write(m, body)
}

func TestAPostedCommentIsOnTheScreenBeforeItLands(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(composed(t, client, "ship it"), "ctrl+enter")

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "ship it") {
		t.Errorf("the comment is not in the conversation:\n%s", out)
	}
	if !strings.Contains(out, "posting") {
		t.Error("the comment does not say it is still on its way")
	}
	if got := client.written(); len(got) != 1 || got[0] != "PR_412: ship it" {
		t.Errorf("wrote %v, want one comment on PR_412", got)
	}
}

func TestACommentThatLandsLosesItsMarkerAndSaysSo(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(composed(t, client, "ship it"), "ctrl+enter")

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "ship it") {
		t.Fatalf("the comment left the conversation when it landed:\n%s", out)
	}
	if strings.Contains(out, "posting") {
		t.Error("the comment still says it is on its way after GitHub confirmed it")
	}
	if !strings.Contains(out, "Posted") {
		t.Error("nothing on the bar says the comment landed")
	}
}

func TestAFailedPostTakesTheCardBackAndKeepsTheWords(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), postErr: errors.New("502 Bad Gateway")}

	m := press(composed(t, client, "ship it"), "ctrl+enter")
	out := stripANSI(render(t, m))

	if n := strings.Count(out, "ship it"); n != 1 {
		t.Errorf("%q appears %d times, want it only in the composer:\n%s", "ship it", n, out)
	}
	if !strings.Contains(out, "ctrl+e editor") {
		t.Error("the box did not take the keyboard back with the failed comment")
	}
	if !strings.Contains(out, "502 Bad Gateway") {
		t.Error("the bar does not say why the comment did not post")
	}
}

func TestARefreshDoesNotTakeAwayACommentStillInFlight(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(composed(t, client, "ship it"), "ctrl+enter")
	m = press(m, "s")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "ship it") {
		t.Errorf("the refresh dropped a comment still on its way:\n%s", out)
	}
}

func TestQIsALetterWhileACommentIsBeingWritten(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := composed(t, client, "quick question")

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "quick question") {
		t.Errorf("the root ate the letters:\n%s", out)
	}
	if strings.Contains(out, "My PRs") {
		t.Error("a letter in the composer left the detail screen")
	}
}

func TestTheHelpKeyIsPunctuationWhileACommentIsBeingWritten(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := composed(t, client, "does this work?")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "does this work?") {
		t.Errorf("? opened the help instead of typing:\n%s", out)
	}
}

func TestQAndTheHelpKeyAreLettersInTheListsSearchBar(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)
	m = settle(m, keyMsg("/"), keyMsg("q"), keyMsg("?"))

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "/ q?") {
		t.Errorf("the root ate the letters:\n%s", out)
	}
}

func TestTheStatusBarNamesTheSearchKeysWhileTheBarIsOpen(t *testing.T) {
	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)

	if out := stripANSI(render(t, m)); !strings.Contains(out, "search") {
		t.Errorf("the bar does not name the search key:\n%s", out)
	}

	out := stripANSI(render(t, settle(m, keyMsg("/"))))
	if !strings.Contains(out, "clear") || !strings.Contains(out, "apply") {
		t.Errorf("the status bar does not name the bar's own keys:\n%s", out)
	}
	if strings.Contains(out, "copy link") {
		t.Errorf("the status bar names a key that types rather than acts:\n%s", out)
	}
}

func TestCtrlCStillQuitsFromTheComposer(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := composed(t, client, "half written")
	_, cmd := m.Update(keyMsg("ctrl+c"))

	if cmd == nil {
		t.Fatal("ctrl+c did nothing in the composer")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Errorf("ctrl+c sent %T, want a quit", msg)
	}
}

func TestTheHelpOverlayStaysInsideTheFrame(t *testing.T) {
	sizes := []struct{ width, height int }{
		{width: 200, height: 50},
		{width: 120, height: 40},
		{width: 100, height: 30},
		{width: 80, height: 24},
		{width: app.MinWidth, height: app.MinHeight},
	}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			client := &fakeSearcher{prs: samplePRs()}
			client.serveDetail("PR_412", "Caps the backoff at 30s.")
			m := press(loaded(t, client, size.width, size.height), "enter", "?")

			lines := strings.Split(render(t, m), "\n")
			if len(lines) != size.height {
				t.Fatalf("frame is %d lines, want %d", len(lines), size.height)
			}
			for i, line := range lines {
				if w := lipgloss.Width(line); w > size.width {
					t.Errorf("line %d is %d cells wide, want no more than %d", i, w, size.width)
				}
			}

			out := stripANSI(render(t, m))
			at := strings.Index(out, "╭─Keys")
			if at < 0 {
				t.Fatal("the help overlay is not on the frame")
			}
			if head := out[at : strings.Index(out[at:], "\n")+at]; !strings.Contains(head, "╮") {
				t.Errorf("the overlay's top border has no right corner: %q", head)
			}
		})
	}
}

func TestTheViewerReachesAScreenAlreadyOpen(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), viewer: gh.ViewerResult{Viewer: gh.Actor{Login: "drucial"}}}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := app.New(testConfig(), client, testSurface, nil)
	var viewer, rest []tea.Msg
	for _, msg := range immediate(m.Init()) {
		if fmt.Sprintf("%T", msg) == "app.viewerFetchedMsg" {
			viewer = append(viewer, msg)
			continue
		}
		rest = append(rest, msg)
	}
	if len(viewer) != 1 {
		t.Fatalf("startup produced %d viewer responses, want one", len(viewer))
	}

	opened := press(settle(m, append(rest, tea.WindowSizeMsg{Width: 160, Height: 40})...), "enter")
	if out := stripANSI(render(t, opened)); strings.Contains(out, "drucial · write a comment") {
		t.Fatal("the viewer reached the screen before the response was applied")
	}

	m2 := settle(opened, viewer...)
	if out := stripANSI(render(t, m2)); !strings.Contains(out, "drucial · write a comment") {
		t.Errorf("the comment box is still headed by nobody:\n%s", out)
	}
}

func TestTheHelpOverlaySaysWhenItCannotShowEverything(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "body")

	roomy := stripANSI(render(t, press(loaded(t, client, 120, 34), "enter", "?")))
	for _, want := range []string{"comment", "reply", "quote reply", "$EDITOR", "quit from anywhere", "sync"} {
		if !strings.Contains(roomy, want) {
			t.Errorf("a frame with room for the help is missing %q", want)
		}
	}
	if strings.Contains(roomy, "more keys than this frame") {
		t.Error("a frame with room for the help claims it is short of room")
	}

	cramped := stripANSI(render(t, press(loaded(t, client, app.MinWidth, app.MinHeight), "enter", "?")))
	if !strings.Contains(cramped, "more keys than this frame") {
		t.Errorf("the overlay drops bindings without saying so:\n%s", cramped)
	}
}

func (f *fakeSearcher) serveThread(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.details == nil {
		f.details = make(map[string]gh.PullRequestDetail)
	}
	held := f.details[id]
	held.Threads = []gh.ReviewThread{{
		ID: "RT_1", Path: "internal/gh/client.go", Line: 42,
		Side: gh.SideRight, CanReply: true, CanResolve: true,
		Comments: []gh.Comment{{
			Kind: gh.CommentThread, ID: "RC_1", Author: gh.Actor{Login: "nkr"},
			CreatedAt: time.Now(), Body: "This backs off forever.",
		}},
	}}
	f.details[id] = held
}

func answering(t *testing.T, client *fakeSearcher, body string) tea.Model {
	t.Helper()

	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveThread("PR_412")

	m := press(loaded(t, client, 160, 40), "enter", "2", "}", "r")
	return write(m, body)
}

func TestAPostedReplyIsInItsThreadBeforeItLands(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(answering(t, client, "capped it"), "ctrl+enter")

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "capped it") {
		t.Errorf("the reply is not on the screen:\n%s", out)
	}
	if !strings.Contains(out, "posting") {
		t.Error("the reply does not say it is still on its way")
	}
	if got := client.answered(); len(got) != 1 || got[0] != "RT_1: capped it" {
		t.Errorf("sent %v, want the reply addressed to the thread", got)
	}
	if got := client.written(); len(got) != 0 {
		t.Errorf("sent %v as a top-level comment, want the reply on the thread alone", got)
	}
}

func TestAReplyThatLandsLosesItsMarkerAndSaysSo(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(answering(t, client, "capped it"), "ctrl+enter")

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "capped it") {
		t.Errorf("the reply left the screen when it landed:\n%s", out)
	}
	if strings.Contains(out, "posting") {
		t.Error("the reply still says it is on its way")
	}
	if !strings.Contains(lastLine(render(t, m)), "Replied") {
		t.Errorf("status bar = %q, want the reply reported", strings.TrimSpace(lastLine(render(t, m))))
	}
}

func TestAFailedReplyGoesBackToItsThread(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), postErr: errors.New("502 Bad Gateway")}

	m := press(answering(t, client, "capped it"), "ctrl+enter")
	out := stripANSI(render(t, m))

	if strings.Count(out, "capped it") != 1 {
		t.Errorf("the words are not in exactly one place:\n%s", out)
	}
	if !strings.Contains(out, "write a reply") {
		t.Error("the box did not reopen on the thread")
	}
	if strings.Contains(out, "posting") {
		t.Error("the reply that failed is still on the thread")
	}
	if !strings.Contains(lastLine(render(t, m)), "502 Bad Gateway") {
		t.Errorf("status bar = %q, want the reason on it", strings.TrimSpace(lastLine(render(t, m))))
	}
}

func TestASyncDoesNotTakeAwayAReplyStillInFlight(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(answering(t, client, "capped it"), "ctrl+enter")
	m = press(m, "s")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "capped it") {
		t.Errorf("the sync dropped a reply still on its way:\n%s", out)
	}
}

func settling(t *testing.T, client *fakeSearcher) tea.Model {
	t.Helper()

	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveThread("PR_412")

	return press(loaded(t, client, 160, 40), "enter", "2", "}")
}

func TestAResolvedThreadReadsResolvedBeforeItLands(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(settling(t, client), "x")

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "resolved") {
		t.Errorf("the thread does not read as settled:\n%s", out)
	}
	if strings.Contains(out, "This backs off forever.") {
		t.Error("the settled thread is still showing its comments")
	}
	if got := client.resolved(); len(got) != 1 || got[0] != "RT_1: true" {
		t.Errorf("sent %v, want the resolve addressed to the thread", got)
	}
}

func TestAResolveThatLandsSaysSo(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}

	m := press(settling(t, client), "x")

	if !strings.Contains(lastLine(render(t, m)), "Resolved") {
		t.Errorf("status bar = %q, want the resolve reported", strings.TrimSpace(lastLine(render(t, m))))
	}
	if out := stripANSI(render(t, m)); !strings.Contains(out, "resolved") {
		t.Errorf("the thread came back open after landing:\n%s", out)
	}
}

func TestAFailedResolvePutsTheThreadBack(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), postErr: errors.New("502 Bad Gateway")}

	m := press(settling(t, client), "x")

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "This backs off forever.") {
		t.Errorf("the thread stayed settled after the write failed:\n%s", out)
	}
	if !strings.Contains(lastLine(render(t, m)), "502 Bad Gateway") {
		t.Errorf("status bar = %q, want the reason on it", strings.TrimSpace(lastLine(render(t, m))))
	}
}

func TestASyncDoesNotUndoAResolveStillInFlight(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(press(settling(t, client), "x"), "s")

	if out := stripANSI(render(t, m)); !strings.Contains(out, "resolved") {
		t.Errorf("the sync opened a thread whose resolve is still on its way:\n%s", out)
	}
}

func TestVFetchesTheDiffAndLandsOnTheThread(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveFiles(412, []gh.ChangedFile{{
		Path: "internal/gh/client.go", Status: gh.FileModified, Additions: 1,
		Hunks: []gh.Hunk{{
			Header: "@@ -40,2 +40,3 @@",
			Lines: []gh.DiffLine{
				{Kind: gh.DiffContext, Old: 40, New: 40, Content: "for {"},
				{Kind: gh.DiffAdded, New: 42, Content: "delay = min(delay*2, fetchTimeout)"},
			},
		}},
	}})

	m := press(settling(t, client), "v")

	out := stripANSI(render(t, m))
	if !strings.Contains(out, "internal/gh/client.go:42") {
		t.Errorf("v never reached the thread in the diff:\n%s", out)
	}
	if !strings.Contains(out, "delay = min(delay*2, fetchTimeout)") {
		t.Errorf("the diff around the thread is not on the screen:\n%s", out)
	}
}

func TestAThreadWhoseFileIsNotInTheDiffSaysSo(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveFiles(412, []gh.ChangedFile{{
		Path: "internal/store/store.go", Status: gh.FileModified,
		Hunks: []gh.Hunk{{
			Header: "@@ -1,1 +1,1 @@",
			Lines:  []gh.DiffLine{{Kind: gh.DiffContext, Old: 1, New: 1, Content: "package store"}},
		}},
	}})

	m := press(settling(t, client), "v")

	if got := lastLine(render(t, m)); !strings.Contains(got, "internal/gh/client.go is not in the diff") {
		t.Errorf("status bar = %q, want it to say the file is not in the diff", strings.TrimSpace(got))
	}
}

func TestASecondXWhileTheResolveIsOutSendsNothing(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.holdPosts()

	m := press(press(settling(t, client), "x"), "x")

	if got := client.resolved(); len(got) != 1 {
		t.Errorf("sent %v, want one write on the wire", got)
	}
	if out := stripANSI(render(t, m)); !strings.Contains(out, "resolved") {
		t.Errorf("the second press undid the first on the page:\n%s", out)
	}
}

func (f *fakeSearcher) Branches(_ context.Context, _, query string) (gh.BranchResult, error) {
	f.mu.Lock()
	f.branchQueries = append(f.branchQueries, query)
	staged, err := slices.Clone(f.branches), f.branchErr
	f.mu.Unlock()

	if err != nil {
		return gh.BranchResult{}, err
	}

	out := make([]string, 0, len(staged))
	for _, b := range staged {
		if strings.Contains(strings.ToLower(b), strings.ToLower(query)) {
			out = append(out, b)
		}
	}
	return gh.BranchResult{Query: query, Default: "main", Branches: out}, nil
}

func (f *fakeSearcher) failBranches(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.branchErr = err
}

func (f *fakeSearcher) serveBranches(names ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.branches = names
}

func (f *fakeSearcher) searches() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.branchQueries)
}

func (f *fakeSearcher) SetBase(_ context.Context, prID, base string) (gh.BaseResult, error) {
	f.mu.Lock()
	f.retargeted = append(f.retargeted, prID+": "+base)
	err, hold := f.postErr, f.postHold
	f.mu.Unlock()

	time.Sleep(hold)

	if err != nil {
		return gh.BaseResult{}, err
	}

	f.mu.Lock()
	for i := range f.prs {
		if f.prs[i].ID == prID {
			f.prs[i].BaseRefName = base
			if f.retargetedFiles > 0 {
				f.prs[i].ChangedFiles = f.retargetedFiles
			}
		}
	}
	if d, ok := f.details[prID]; ok {
		d.BaseRefName = base
		d.BehindBy = 0
		f.details[prID] = d
	}
	f.mu.Unlock()

	return gh.BaseResult{BaseRefName: base}, nil
}

func (f *fakeSearcher) retargets() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.retargeted)
}

func (f *fakeSearcher) Merge(_ context.Context, prID string, opts gh.MergeOptions) (gh.MergeResult, error) {
	f.mu.Lock()
	f.merged = append(f.merged, opts)
	err, hold := f.postErr, f.postHold
	f.mu.Unlock()

	time.Sleep(hold)

	if err != nil {
		return gh.MergeResult{}, err
	}

	f.mu.Lock()
	for i := range f.prs {
		if f.prs[i].ID == prID {
			f.prs[i].State = gh.PRStateMerged
		}
	}
	if d, ok := f.details[prID]; ok {
		d.State = gh.PRStateMerged
		f.details[prID] = d
	}
	f.mu.Unlock()

	state := f.mergeState
	if state == "" {
		state = gh.PRStateMerged
	}
	return gh.MergeResult{State: state}, nil
}

func (f *fakeSearcher) DeleteRef(_ context.Context, refID string) error {
	f.mu.Lock()
	f.deletedRefs = append(f.deletedRefs, refID)
	err := f.deleteErr
	f.mu.Unlock()
	return err
}

func (f *fakeSearcher) merges() []gh.MergeOptions {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.merged)
}

func (f *fakeSearcher) deletes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.deletedRefs)
}

func TestSelectingAJobFetchesItsMetadataAndLogOnce(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "body")
	client.mu.Lock()
	d := client.details["PR_412"]
	d.Rollup = gh.CheckRollup{Checks: []gh.Check{{Name: "test", Workflow: "CI", State: gh.CheckStateSuccess, JobID: 9001}}}
	client.details["PR_412"] = d
	client.mu.Unlock()
	client.servedJob(9001, gh.Job{
		ID: 9001, Name: "test", State: gh.CheckStateSuccess,
		Steps: []gh.JobStep{{Number: 1, Name: "Run tests", State: gh.CheckStateSuccess}},
	}, "2026-08-19T14:00:00Z ok\n")

	m := press(loaded(t, client, 160, 40), "enter", "]", "]")
	m = settleJob(m, d.Rollup.Checks[0], false)
	if got := client.askedJobs(); !slices.Equal(got, []int64{9001}) {
		t.Fatalf("job asks = %v, want [9001]", got)
	}
	if out := render(t, m); !strings.Contains(out, "Run tests") {
		t.Errorf("the landed job did not reach the pane:\n%s", out)
	}

	press(m, "[", "]")
	if got := client.askedJobs(); !slices.Equal(got, []int64{9001}) {
		t.Errorf("reopening the cached job fetched again: %v", got)
	}
}

func TestARunningJobDoesNotAskForABlobThatDoesNotExistYet(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "body")
	client.mu.Lock()
	d := client.details["PR_412"]
	d.Rollup = gh.CheckRollup{Checks: []gh.Check{{Name: "test", Workflow: "CI", State: gh.CheckStatePending, JobID: 9001}}}
	client.details["PR_412"] = d
	client.mu.Unlock()
	client.servedJob(9001, gh.Job{
		ID: 9001, Name: "test", State: gh.CheckStatePending,
		Steps: []gh.JobStep{{
			Number: 1, Name: "Run tests", State: gh.CheckStatePending,
			StartedAt: time.Date(2026, 8, 19, 14, 0, 0, 0, time.UTC),
		}},
	}, "not available yet")

	m := press(loaded(t, client, 160, 40), "enter", "]", "]")
	m = settleJob(m, d.Rollup.Checks[0], false)
	if got := client.askedJobs(); !slices.Equal(got, []int64{9001}) {
		t.Fatalf("metadata asks = %v, want [9001]", got)
	}
	if got := client.askedJobLogs(); len(got) != 0 {
		t.Errorf("running job asked for logs: %v", got)
	}
	m = press(m, "2")
	before := render(t, m)
	if !strings.Contains(before, "Run tests") || !strings.Contains(before, "Log output is not available yet") ||
		strings.Contains(before, "No log output") {
		t.Errorf("running job pane:\n%s", before)
	}
	m = press(m, "space")
	if after := render(t, m); after != before {
		t.Error("a running step without available logs expanded")
	}

	client.mu.Lock()
	d.Rollup.Checks[0].State = gh.CheckStateSuccess
	client.details["PR_412"] = d
	client.mu.Unlock()
	client.servedJob(9001, gh.Job{
		ID: 9001, Name: "test", State: gh.CheckStateSuccess,
		Steps: []gh.JobStep{{Number: 1, Name: "Run tests", State: gh.CheckStateSuccess}},
	}, "2026-08-19T14:00:00Z completed output\n")
	m = press(m, "s")
	m = settleJob(m, d.Rollup.Checks[0], true)
	if got := client.askedJobs(); !slices.Equal(got, []int64{9001, 9001}) {
		t.Errorf("completion metadata asks = %v, want the same job refetched", got)
	}
	if got := client.askedJobLogs(); !slices.Equal(got, []int64{9001}) {
		t.Errorf("completion log asks = %v, want [9001]", got)
	}
	m = press(m, "space")
	if out := render(t, m); !strings.Contains(out, "completed output") {
		t.Errorf("completed log did not replace running metadata:\n%s", out)
	}
}

func TestRerunningASelectedFailedCheckCallsGitHubAndToasts(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "body")
	client.mu.Lock()
	d := client.details["PR_412"]
	d.Rollup = gh.CheckRollup{Checks: []gh.Check{{
		Name: "test", Workflow: "CI", State: gh.CheckStateFailure, JobID: 9001,
	}}}
	client.details["PR_412"] = d
	client.mu.Unlock()

	m := press(loaded(t, client, 160, 40), "enter", "]", "]", "r")
	if got := client.askedReruns(); !slices.Equal(got, []int64{9001}) {
		t.Fatalf("reruns = %v, want [9001]", got)
	}
	out := render(t, m)
	if !strings.Contains(out, "Rerunning CI / test") || !strings.Contains(out, "rerunning") {
		t.Errorf("accepted rerun did not stay optimistic through GitHub's indexing gap:\n%s", out)
	}
}

func TestARefusedCheckRerunReleasesTheKeyAndReportsWhy(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), rerunErr: errors.New("actions write denied")}
	client.serveDetail("PR_412", "body")
	client.mu.Lock()
	d := client.details["PR_412"]
	d.Rollup = gh.CheckRollup{Checks: []gh.Check{{
		Name: "test", Workflow: "CI", State: gh.CheckStateFailure, JobID: 9001,
	}}}
	client.details["PR_412"] = d
	client.mu.Unlock()

	m := press(loaded(t, client, 160, 40), "enter", "]", "]", "r")
	if out := render(t, m); !strings.Contains(out, "Could not rerun CI / test: actions write denied") {
		t.Errorf("failure toast:\n%s", out)
	}
	_ = press(m, "r")
	if got := client.askedReruns(); !slices.Equal(got, []int64{9001, 9001}) {
		t.Errorf("r stayed locked after failure: %v", got)
	}
}

func TestAJobLogFailureKeepsTheFetchedStepMetadata(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), jobLogErr: errors.New("log expired")}
	client.serveDetail("PR_412", "body")
	client.mu.Lock()
	d := client.details["PR_412"]
	d.Rollup = gh.CheckRollup{Checks: []gh.Check{{Name: "test", Workflow: "CI", State: gh.CheckStateFailure, JobID: 9001}}}
	client.details["PR_412"] = d
	client.mu.Unlock()
	client.servedJob(9001, gh.Job{
		ID: 9001, Name: "test", State: gh.CheckStateFailure,
		Steps: []gh.JobStep{{Number: 1, Name: "Run tests", State: gh.CheckStateFailure}},
	}, "")

	m := press(loaded(t, client, 160, 40), "enter", "]", "]")
	m = settleJob(m, d.Rollup.Checks[0], false)
	out := render(t, m)
	if !strings.Contains(out, "Run tests") || !strings.Contains(out, "Log output is unavailable: log expired") {
		t.Errorf("partial job did not keep its metadata:\n%s", out)
	}
}

func TestAJobFetchFailureStaysInTheSelectedPane(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs(), jobErr: errors.New("no such host")}
	client.serveDetail("PR_412", "body")
	client.mu.Lock()
	d := client.details["PR_412"]
	d.Rollup = gh.CheckRollup{Checks: []gh.Check{{Name: "test", Workflow: "CI", State: gh.CheckStateFailure, JobID: 9001}}}
	client.details["PR_412"] = d
	client.mu.Unlock()

	m := press(loaded(t, client, 160, 40), "enter", "]", "]")
	m = settleJob(m, d.Rollup.Checks[0], false)
	if out := render(t, m); !strings.Contains(out, "Could not load the job log: no such host") {
		t.Errorf("job failure did not reach the pane:\n%s", out)
	}

	client.mu.Lock()
	client.jobErr = nil
	client.mu.Unlock()
	client.servedJob(9001, gh.Job{
		ID: 9001, Name: "test", State: gh.CheckStateFailure,
		Steps: []gh.JobStep{{Number: 1, Name: "Run tests", State: gh.CheckStateFailure}},
	}, "2026-08-19T14:00:00Z retry reached GitHub\n")
	m = press(m, "s")
	m = settleJob(m, d.Rollup.Checks[0], false)
	if got := client.askedJobs(); !slices.Equal(got, []int64{9001, 9001}) {
		t.Errorf("retry asks = %v", got)
	}
	if out := render(t, m); !strings.Contains(out, "retry reached GitHub") {
		t.Errorf("sync did not retry the failed job fetch:\n%s", out)
	}
}

func TestSideBySideInAPaneTooNarrowSaysHowShortItIs(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")
	client.serveFiles(412, sampleFiles())

	m := press(loaded(t, client, 90, 40), "enter", "]", "]", "]", "|")

	got := lastLine(render(t, m))
	if !strings.Contains(got, "Side by side needs") || !strings.Contains(got, "more columns in the pane") {
		t.Errorf("status bar = %q, want it to name the columns the pane is short", strings.TrimSpace(got))
	}
}

// Read off a rendered cell: a slot goes over the wire as its own SGR code, not 38;2;r;g;b.
func sgrParams(s lipgloss.Style) string {
	out := s.Render("x")
	end := strings.Index(out, "m")
	if end < 0 {
		return ""
	}
	return out[len("\x1b["):end]
}

func (f *fakeSearcher) askedRunReruns() []runRerun {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.runReruns)
}

func bulkChecksClient(t *testing.T, runErr error) *fakeSearcher {
	t.Helper()

	client := &fakeSearcher{prs: samplePRs(), runRerunErr: runErr}
	client.serveDetail("PR_412", "body")
	client.mu.Lock()
	d := client.details["PR_412"]
	d.Rollup = gh.CheckRollup{Checks: []gh.Check{
		{Name: "lint", Workflow: "CI", State: gh.CheckStateSuccess, JobID: 9001, RunID: 77},
		{Name: "test", Workflow: "CI", State: gh.CheckStateFailure, JobID: 9002, RunID: 77},
	}}
	client.details["PR_412"] = d
	client.mu.Unlock()
	return client
}

func TestRerunningAWorkflowsFailedJobsGoesOutAndReportsItself(t *testing.T) {
	client := bulkChecksClient(t, nil)

	m := press(loaded(t, client, 160, 40), "enter", "]", "]", "k", "r")

	got := client.askedRunReruns()
	if len(got) != 1 || got[0].runID != 77 || got[0].all {
		t.Fatalf("run reruns = %+v, want one failed-jobs call on run 77", got)
	}
	if out := render(t, m); !strings.Contains(out, "Rerunning failed jobs in CI") {
		t.Errorf("the bulk rerun said nothing about what it did:\n%s", out)
	}
}

func TestRerunningEveryJobInAWorkflowGoesOutAndReportsItself(t *testing.T) {
	client := bulkChecksClient(t, nil)

	m := press(loaded(t, client, 160, 40), "enter", "]", "]", "k", "R")

	got := client.askedRunReruns()
	if len(got) != 1 || got[0].runID != 77 || !got[0].all {
		t.Fatalf("run reruns = %+v, want one all-jobs call on run 77", got)
	}
	if out := render(t, m); !strings.Contains(out, "Rerunning all jobs in CI") {
		t.Errorf("the bulk rerun did not name which of the two it made:\n%s", out)
	}
}

func TestARefusedWorkflowRerunReleasesEveryMarkItMade(t *testing.T) {
	client := bulkChecksClient(t, errors.New("actions write denied"))

	m := press(loaded(t, client, 160, 40), "enter", "]", "]", "k", "R")

	out := render(t, m)
	if !strings.Contains(out, "Could not rerun CI") {
		t.Errorf("the refusal said nothing:\n%s", out)
	}
	if strings.Contains(out, "rerunning") {
		t.Errorf("a refused bulk rerun left its marks on the column:\n%s", out)
	}
}
