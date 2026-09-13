// Package app is the root Bubble Tea model, dividing the frame between the screens and the status bar.
package app

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/config"
	"github.com/praxis-labs-io/zen-octo/internal/gh"
	"github.com/praxis-labs-io/zen-octo/internal/store"
	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
	"github.com/praxis-labs-io/zen-octo/internal/tui/keys"
	"github.com/praxis-labs-io/zen-octo/internal/tui/list"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
	"github.com/praxis-labs-io/zen-octo/internal/tui/syntax"
	"github.com/praxis-labs-io/zen-octo/internal/tui/theme"
)

// GitHub is the client the UI calls.
type GitHub interface {
	Viewer(ctx context.Context) (gh.ViewerResult, error)
	SearchPullRequests(ctx context.Context, query string, limit int) (gh.SearchResult, error)
	PullRequest(ctx context.Context, id, headRef string) (gh.DetailResult, error)
	Pulse(ctx context.Context, id string) (gh.PulseResult, error)
	PullRequestFiles(ctx context.Context, prID, repo string, number, changedFiles int) (gh.FilesResult, error)
	CommitFiles(ctx context.Context, repo, sha string) (gh.FilesResult, error)
	Job(ctx context.Context, repo string, jobID int64) (gh.Job, error)
	JobLogs(ctx context.Context, repo string, jobID int64) ([]byte, error)
	RerunJob(ctx context.Context, repo string, jobID int64) (time.Time, error)

	RerunFailedJobs(ctx context.Context, repo string, runID int64) error
	RerunAllJobs(ctx context.Context, repo string, runID int64) error
	SetFileViewed(ctx context.Context, prID, path string, viewed bool) error
	AddComment(ctx context.Context, subjectID, body string) (gh.CommentResult, error)
	AddReply(ctx context.Context, threadID, body string) (gh.CommentResult, error)
	SetThreadResolved(ctx context.Context, threadID string, resolved bool) (gh.ThreadResult, error)

	// SetReaction toggles content on any reactable node, the pull request included.
	SetReaction(ctx context.Context, subjectID string, content gh.ReactionContent, on bool) (gh.ReactionResult, error)

	UpdateComment(ctx context.Context, kind gh.CommentKind, id, body string) (gh.CommentResult, error)
	DeleteComment(ctx context.Context, kind gh.CommentKind, id string) error

	SetBody(ctx context.Context, prID, body string) (gh.BodyResult, error)
	RepoMeta(ctx context.Context, repo string) (gh.RepoMetaResult, error)
	SetLabels(ctx context.Context, prID string, labelIDs []string) (gh.LabelsResult, error)
	SetState(ctx context.Context, prID string, to gh.PRTransition) (gh.PRStateResult, error)
	SetAssignees(ctx context.Context, prID string, assigneeIDs []string) (gh.AssigneesResult, error)
	SetBase(ctx context.Context, prID, base string) (gh.BaseResult, error)

	Merge(ctx context.Context, prID string, opts gh.MergeOptions) (gh.MergeResult, error)
	DeleteRef(ctx context.Context, refID string) error

	Branches(ctx context.Context, repo, query string) (gh.BranchResult, error)

	RequestReviews(ctx context.Context, repo string, number int, logins []string) error
	RemoveReviewRequests(ctx context.Context, repo string, number int, logins []string) error
}

type viewerFetchedMsg struct {
	res gh.ViewerResult
}

type viewerFailedMsg struct{}

type sectionFetchedMsg struct {
	index int
	res   gh.SearchResult
}

type sectionFailedMsg struct {
	index int
	err   error
}

type detailFetchedMsg struct {
	id  string
	res gh.DetailResult
}

type detailFailedMsg struct {
	id  string
	err error
}

type filesFetchedMsg struct {
	id  string
	res gh.FilesResult
}

type filesFailedMsg struct {
	id  string
	err error
}

type fileViewedMsg struct {
	id  string
	key string
}

type fileViewFailedMsg struct {
	id   string
	key  string
	path string
	err  error
}

type commitFilesFetchedMsg struct {
	sha string
	res gh.FilesResult
}

type commitFilesFailedMsg struct {
	sha string
	err error
}

type jobFetchedMsg struct {
	id  int64
	job gh.Job
	log []byte
}

type jobFailedMsg struct {
	id  int64
	job gh.Job
	err error
}

type commentPostedMsg struct {
	id  string
	key string
	res gh.CommentResult
}

type commentFailedMsg struct {
	id   string
	key  string
	body string
	err  error
}

type replyPostedMsg struct {
	id  string
	key string
	res gh.CommentResult
}

type replyFailedMsg struct {
	id     string
	key    string
	thread string
	body   string
	err    error
}

type threadResolvedMsg struct {
	id  string
	key string
	res gh.ThreadResult
}

type resolveFailedMsg struct {
	id       string
	key      string
	resolved bool
	err      error
}

// Bounds a request so a half-open socket cannot leave the UI spinning forever.
const fetchTimeout = 30 * time.Second

const statusBarHeight = 1

type screen int

const (
	screenList screen = iota
	screenDetail
)

type Model struct {
	client       GitHub
	releaseCheck ReleaseCheck
	theme        theme.Theme
	syntax       syntax.Syntax
	limit        int

	store store.Store

	screen screen
	list   list.Model
	detail prview.Model
	status comp.StatusBar
	toasts comp.Toasts
	help   help.Model

	notice       string
	newerRelease string
	showHelp     bool

	// Held here because the screen is rebuilt on every open and the terminal answers once.
	chords bool

	refreshing []int

	detailRefreshing detailRefresh
	refreshSpin      comp.Spinner

	poller poller

	width  int
	height int
}

// Legs are held apart rather than counted, so an unrelated response cannot fill a missing leg's slot.
type detailRefresh struct {
	detail leg
	files  leg
	commit leg
}

type leg struct {
	key    string
	done   bool
	failed bool
}

func (l leg) started() bool { return l.key != "" }
func (l leg) running() bool { return l.started() && !l.done }

func (l *leg) claim(key string, err error) bool {
	if !l.running() || key != l.key {
		return false
	}
	l.done, l.failed = true, err != nil
	return true
}

func (r detailRefresh) running() bool {
	return r.detail.running() || r.files.running() || r.commit.running()
}

type refreshLeg int

const (
	legDetail refreshLeg = iota
	legFiles
	legCommit
)

func New(cfg *config.Config, client GitHub, surface theme.Surface, check ReleaseCheck) Model {
	colors := theme.NewOverrides(cfg.Theme.Colors, cfg.Theme.Named)
	colorErr := colors.Validate()
	if colorErr != nil {
		colors = theme.NewOverrides(nil, cfg.Theme.Named)
	}

	th := colors.Resolve(surface, cfg.Transparent)

	syntaxName := cmp.Or(cfg.SyntaxTheme, th.Syntax)
	syn, syntaxOK := syntax.New(syntaxName)

	h := help.New()
	h.Styles = helpStyles(th)

	m := Model{
		client: client,
		theme:  th,
		syntax: syn,
		limit:  cfg.Defaults.PRsLimit,
		store:  store.New(cfg.PRSections),
		list:   list.New(th),
		status: comp.NewStatusBar(th),
		help:   h,

		refreshSpin: comp.NewSpinner(th),
	}
	if cfg.ChecksForUpdates() {
		m.releaseCheck = check
	}
	m.store.BeginAll()
	m.list.SetSections(m.store.Sections())

	switch {
	case colorErr != nil:
		m.notice = colorErr.Error()
	case cfg.Theme.Named != "":
		m.notice = fmt.Sprintf("Theme names are gone: the chrome now follows your terminal. "+
			"Drop %q, or set colors under theme: to pin any it gets wrong. Known: %s",
			cfg.Theme.Named, strings.Join(theme.Keys(), ", "))
	case !syntaxOK:
		m.notice = fmt.Sprintf("Unknown syntax theme %q, using Chroma's default. Known: %s",
			syntaxName, strings.Join(syntax.Names(), ", "))
	}
	return m
}

// Init starts the list and the background poll, fetches the viewer and every section, and asks for a newer release.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.list.Init(), m.fetchViewer(), armPoll(), checkRelease(m.releaseCheck)}
	for i, section := range m.store.Sections() {
		cmds = append(cmds, m.fetchSection(i, section.Filters))
	}
	return tea.Batch(cmds...)
}

func (m Model) fetchViewer() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.Viewer(ctx)
		if err != nil {
			return viewerFailedMsg{}
		}
		return viewerFetchedMsg{res: res}
	}
}

func (m Model) postComment(msg prview.PostCommentMsg) (tea.Model, tea.Cmd) {
	key := m.store.PendingComment(msg.ID, gh.Comment{
		Kind:      gh.CommentIssue,
		Author:    m.store.Viewer(),
		CreatedAt: time.Now(),
		Body:      msg.Body,
	})

	shown := m.detail.SetDetail(m.store.Detail(msg.ID))
	return m, tea.Batch(shown, m.sendComment(msg.ID, key, msg.Body))
}

func (m Model) sendComment(id, key, body string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.AddComment(ctx, id, body)
		if err != nil {
			return commentFailedMsg{id: id, key: key, body: body, err: err}
		}
		return commentPostedMsg{id: id, key: key, res: res}
	}
}

func (m Model) commentLanded(msg commentPostedMsg) (tea.Model, tea.Cmd) {
	m.store.PendingApplied(msg.id, msg.key, msg.res)

	toast := m.toasts.Show(comp.ToastSuccess, "Posted")
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}

func (m Model) commentFailed(msg commentFailedMsg) (tea.Model, tea.Cmd) {
	m.store.PendingReverted(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastError, "Could not post the comment: "+msg.err.Error())

	if !m.showing(msg.id) {
		return m, toast
	}

	shown := m.detail.SetDetail(m.store.Detail(msg.id))
	restored := m.detail.RestoreDraft(msg.body)
	m.resize()
	return m, tea.Batch(shown, restored, toast)
}

func (m Model) postReply(msg prview.PostReplyMsg) (tea.Model, tea.Cmd) {
	key := m.store.PendingReply(msg.ID, msg.ThreadID, gh.Comment{
		Author:    m.store.Viewer(),
		CreatedAt: time.Now(),
		Body:      msg.Body,
	})

	shown := m.detail.SetDetail(m.store.Detail(msg.ID))
	return m, tea.Batch(shown, m.sendReply(msg, key))
}

func (m Model) sendReply(msg prview.PostReplyMsg, key string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.AddReply(ctx, msg.ThreadID, msg.Body)
		if err != nil {
			return replyFailedMsg{id: msg.ID, key: key, thread: msg.ThreadID, body: msg.Body, err: err}
		}
		return replyPostedMsg{id: msg.ID, key: key, res: res}
	}
}

func (m Model) replyLanded(msg replyPostedMsg) (tea.Model, tea.Cmd) {
	m.store.PendingApplied(msg.id, msg.key, msg.res)

	toast := m.toasts.Show(comp.ToastSuccess, "Replied")
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}

func (m Model) replyFailed(msg replyFailedMsg) (tea.Model, tea.Cmd) {
	m.store.PendingReverted(msg.id, msg.key)

	toast := m.toasts.Show(comp.ToastError, "Could not post the reply: "+msg.err.Error())
	if !m.showing(msg.id) {
		return m, toast
	}

	shown := m.detail.SetDetail(m.store.Detail(msg.id))
	restored := m.detail.RestoreReply(msg.thread, msg.body)
	m.resize()
	return m, tea.Batch(shown, restored, toast)
}

func (m Model) resolveThread(msg prview.ResolveThreadMsg) (tea.Model, tea.Cmd) {
	key := m.store.PendingResolve(msg.ID, msg.ThreadID, msg.Resolved)

	shown := m.detail.SetDetail(m.store.Detail(msg.ID))
	return m, tea.Batch(shown, m.sendResolve(msg, key))
}

func (m Model) sendResolve(msg prview.ResolveThreadMsg, key string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.SetThreadResolved(ctx, msg.ThreadID, msg.Resolved)
		if err != nil {
			return resolveFailedMsg{id: msg.ID, key: key, resolved: msg.Resolved, err: err}
		}
		return threadResolvedMsg{id: msg.ID, key: key, res: res}
	}
}

func (m Model) resolveLanded(msg threadResolvedMsg) (tea.Model, tea.Cmd) {
	m.store.ResolveApplied(msg.id, msg.key, msg.res)

	said := "Unresolved"
	if msg.res.IsResolved {
		said = "Resolved"
	}

	toast := m.toasts.Show(comp.ToastSuccess, said)
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}

func (m Model) resolveFailed(msg resolveFailedMsg) (tea.Model, tea.Cmd) {
	m.store.ResolveReverted(msg.id, msg.key)

	doing := "unresolve"
	if msg.resolved {
		doing = "resolve"
	}

	toast := m.toasts.Show(comp.ToastError, "Could not "+doing+" the thread: "+msg.err.Error())
	if !m.showing(msg.id) {
		return m, toast
	}
	return m, tea.Batch(m.detail.SetDetail(m.store.Detail(msg.id)), toast)
}

func (m Model) showing(id string) bool {
	return m.screen == screenDetail && m.detail.PullRequest().ID == id
}

// The query is expanded inside the command, so a {{since}} window is measured when the request leaves.
func (m Model) fetchSection(index int, query string) tea.Cmd {
	client, limit := m.client, m.limit
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.SearchPullRequests(ctx, config.ExpandQuery(query, time.Now()), limit)
		if err != nil {
			return sectionFailedMsg{index: index, err: err}
		}
		return sectionFetchedMsg{index: index, res: res}
	}
}

func (m Model) refresh() (tea.Model, tea.Cmd) {
	sections := m.store.Sections()

	var cmds []tea.Cmd
	started := make([]int, 0, len(sections))
	for i, section := range sections {
		started = append(started, i)
		if m.store.Begin(i) {
			cmds = append(cmds, m.fetchSection(i, section.Filters))
		}
	}
	if len(started) == 0 {
		return m, nil
	}

	m.refreshing = started
	m.list.SetSections(m.store.Sections())
	return m, tea.Batch(append(cmds, m.list.Init(), m.refreshSpin.Tick())...)
}

func (m Model) sectionSettled() (tea.Model, tea.Cmd) {
	sections := m.store.Sections()
	m.list.SetSections(sections)

	if len(m.refreshing) == 0 || stillLoading(sections, m.refreshing) {
		return m, nil
	}

	kind, text := refreshSummary(sections, m.refreshing)
	m.refreshing = nil
	return m, m.toasts.Show(kind, text)
}

func stillLoading(sections []store.Section, indices []int) bool {
	for _, i := range indices {
		if sections[i].Status == store.StatusLoading {
			return true
		}
	}
	return false
}

func (m Model) open(pr gh.PullRequest) (tea.Model, tea.Cmd) {
	m.detail = prview.New(m.theme, pr, m.detail.Rail(), m.syntax)
	m.detail.SetChords(m.chords)
	m.detail.SetViewer(m.store.Viewer())
	m.screen = screenDetail
	m.detailRefreshing = detailRefresh{}

	var cmds []tea.Cmd
	if m.store.BeginDetail(pr.ID) {
		cmds = append(cmds, m.fetchDetail(pr.ID, pr.HeadRefName))
	}
	cmds = append(cmds, m.detail.Init())

	cmds = append(cmds, m.detail.SetDetail(m.store.Detail(pr.ID)))
	m.store.UseFiles(pr.ID)
	cmds = append(cmds, m.detail.SetFiles(m.store.Files(pr.ID)))
	m.resize()
	return m, tea.Batch(cmds...)
}

func (m Model) refreshDetail(msg prview.RefreshMsg) (tea.Model, tea.Cmd) {
	pr := m.detail.PullRequest()
	if m.screen != screenDetail || pr.ID != msg.ID {
		return m, nil
	}

	m.store.InvalidateRepoMeta(pr.Repository)
	m.store.InvalidateBranches(pr.Repository)
	cmds := []tea.Cmd{m.detail.SetRepo(store.Repo{})}
	m.detail.SetBranches(store.Branches{})

	started := m.detailRefreshing
	switch {
	case m.store.BeginDetail(msg.ID):
		started.detail = leg{key: msg.ID}
		cmds = append(cmds, m.fetchDetail(msg.ID, pr.HeadRefName))

	case m.store.Detail(msg.ID).Status == store.StatusLoading:
		started.detail = leg{key: msg.ID}
	}
	if msg.Files && m.store.BeginFiles(msg.ID) {
		started.files = leg{key: msg.ID}
		if held := m.store.Files(msg.ID); !held.Loaded {
			cmds = append(cmds, m.detail.SetFiles(held))
		}
		cmds = append(cmds, m.fetchFiles(msg.ID, pr.Repository, pr.Number, pr.ChangedFiles))
	} else if msg.Files && m.store.Files(msg.ID).Status == store.StatusLoading {
		started.files = leg{key: msg.ID}
	}
	if msg.SHA != "" && m.store.BeginCommitFiles(msg.SHA) {
		started.commit = leg{key: msg.SHA}
		if held := m.store.CommitFiles(msg.SHA); !held.Loaded {
			m.detail.SetCommitFiles(msg.SHA, held)
		}
		cmds = append(cmds, m.fetchCommitFiles(pr.Repository, msg.SHA))
	} else if msg.SHA != "" && m.store.CommitFiles(msg.SHA).Status == store.StatusLoading {
		started.commit = leg{key: msg.SHA}
	}
	if m.detail.ShowsChecks() {
		cmds = append(cmds, m.detail.RefreshJob())
	}
	if !started.running() {
		return m, nil
	}

	m.detailRefreshing = started
	return m, tea.Batch(append(cmds, m.detail.Init(), m.refreshSpin.Tick())...)
}

func (m *Model) claim(which refreshLeg, key string, err error) (tea.Cmd, bool) {
	r := &m.detailRefreshing

	var took bool
	switch which {
	case legDetail:
		took = r.detail.claim(key, err)
	case legFiles:
		took = r.files.claim(key, err)
	case legCommit:
		took = r.commit.claim(key, err)
	}
	if !took {
		return nil, false
	}

	if r.running() {
		return nil, true
	}

	kind, text := detailRefreshSummary(*r, m.detail.PullRequest().Number)
	*r = detailRefresh{}
	return m.toasts.Show(kind, text), true
}

func detailRefreshSummary(r detailRefresh, number int) (comp.ToastKind, string) {
	var landed, failed []string
	for _, l := range []struct {
		leg  leg
		name string
	}{
		{r.detail, "#" + strconv.Itoa(number)},
		{r.files, "the diff"},
		{r.commit, "the diff"},
	} {
		switch {
		case !l.leg.started():
		case l.leg.failed:
			failed = append(failed, l.name)
		default:
			landed = append(landed, l.name)
		}
	}

	switch {
	case len(failed) == 0:
		return comp.ToastSuccess, "Refreshed " + strings.Join(landed, " and ")
	case len(landed) == 0:
		return comp.ToastError, "Refresh failed"
	default:
		return comp.ToastError, "Refreshed " + strings.Join(landed, " and ") +
			", " + strings.Join(failed, " and ") + " failed"
	}
}

func (m Model) needFiles(id string) (tea.Model, tea.Cmd) {
	if m.detail.PullRequest().ID != id || !m.store.BeginFiles(id) {
		return m, nil
	}
	pr := m.detail.PullRequest()
	shown := m.detail.SetFiles(m.store.Files(id))
	return m, tea.Batch(shown, m.fetchFiles(id, pr.Repository, pr.Number, pr.ChangedFiles), m.detail.Init())
}

func (m Model) fetchFiles(id, repo string, number, changedFiles int) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.PullRequestFiles(ctx, id, repo, number, changedFiles)
		if err != nil {
			return filesFailedMsg{id: id, err: err}
		}
		return filesFetchedMsg{id: id, res: res}
	}
}

func (m Model) filesSettled(id string, err error) (tea.Model, tea.Cmd) {
	held := m.store.Files(id)

	var owed tea.Cmd
	if err == nil {
		owed = m.correctFiles(id)
	}

	if m.screen != screenDetail || m.detail.PullRequest().ID != id {
		return m, owed
	}

	shown := m.detail.SetFiles(held)
	if cmd, claimed := m.claim(legFiles, id, err); claimed {
		return m, tea.Batch(shown, cmd, owed)
	}
	if err != nil && held.Loaded {
		toast := m.toasts.Show(comp.ToastError, "Could not refresh the diff for #"+strconv.Itoa(m.detail.PullRequest().Number))
		return m, tea.Batch(shown, toast)
	}
	return m, tea.Batch(shown, owed)
}

func (m Model) needCommit(sha string) (tea.Model, tea.Cmd) {
	if m.screen != screenDetail {
		return m, nil
	}

	if held := m.store.CommitFiles(sha); held.Loaded || held.Status == store.StatusLoading {
		m.store.UseCommitFiles(sha)
		m.detail.SetCommitFiles(sha, held)
		return m, m.detail.Init()
	}

	if !m.store.BeginCommitFiles(sha) {
		return m, nil
	}
	m.detail.SetCommitFiles(sha, m.store.CommitFiles(sha))
	return m, tea.Batch(m.fetchCommitFiles(m.detail.PullRequest().Repository, sha), m.detail.Init())
}

func (m Model) fetchCommitFiles(repo, sha string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.CommitFiles(ctx, repo, sha)
		if err != nil {
			return commitFilesFailedMsg{sha: sha, err: err}
		}
		return commitFilesFetchedMsg{sha: sha, res: res}
	}
}

func (m Model) commitFilesSettled(sha string, err error) (tea.Model, tea.Cmd) {
	held := m.store.CommitFiles(sha)
	if m.screen != screenDetail {
		return m, nil
	}

	m.detail.SetCommitFiles(sha, held)
	if cmd, claimed := m.claim(legCommit, sha, err); claimed {
		return m, cmd
	}
	if err != nil && held.Loaded {
		return m, m.toasts.Show(comp.ToastError, "Could not refresh the diff for "+short(sha))
	}
	return m, nil
}

// A running job's log is never fetched: GitHub publishes the blob only once the job finishes.
func (m Model) needJob(id int64, refresh bool) (tea.Model, tea.Cmd) {
	if m.screen != screenDetail || id == 0 {
		return m, nil
	}
	if held := m.store.Job(id); held.Status == store.StatusLoading ||
		(held.Loaded && held.Status != store.StatusFailed && !refresh) {
		m.store.UseJob(id)
		return m, m.detail.SetJobAsync(id, held)
	}
	if !m.store.BeginJob(id) {
		return m, nil
	}
	held := m.store.Job(id)
	var loading tea.Cmd
	if !held.Loaded {
		loading = m.detail.SetJob(id, held)
	}
	return m, tea.Batch(m.fetchJob(m.detail.PullRequest().Repository, id), loading, m.detail.Init())
}

func (m Model) fetchJob(repo string, id int64) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		job, err := client.Job(ctx, repo, id)
		if err != nil {
			return jobFailedMsg{id: id, err: err}
		}
		if job.State == gh.CheckStatePending || job.State == gh.CheckStateExpected {
			return jobFetchedMsg{id: id, job: job}
		}
		log, err := client.JobLogs(ctx, repo, id)
		if err != nil {
			return jobFailedMsg{id: id, job: job, err: err}
		}
		return jobFetchedMsg{id: id, job: job, log: log}
	}
}

func (m Model) jobSettled(id int64, err error, hadLoaded bool) (tea.Model, tea.Cmd) {
	if m.screen != screenDetail {
		return m, nil
	}
	held := m.store.Job(id)
	cmd := m.detail.SetJobAsync(id, held)
	if err != nil && hadLoaded {
		return m, tea.Batch(cmd, m.toasts.Show(comp.ToastError, "Could not refresh the job log"))
	}
	return m, cmd
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func (m Model) fetchDetail(id, headRef string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		res, err := client.PullRequest(ctx, id, headRef)
		if err != nil {
			return detailFailedMsg{id: id, err: err}
		}
		return detailFetchedMsg{id: id, res: res}
	}
}

func (m Model) detailSettled(id string, err error) (tea.Model, tea.Cmd) {
	held := m.store.Detail(id)

	m.list.SetSections(m.store.Sections())

	var owed tea.Cmd
	if err == nil && m.store.StaleDetail(id) {
		owed = m.correctDetail(id)
	}

	if err == nil {
		owed = tea.Batch(owed, m.correctFiles(id))
	}

	if m.screen != screenDetail || m.detail.PullRequest().ID != id {
		return m, owed
	}

	armed := m.detail.SetDetail(held)
	if cmd, claimed := m.claim(legDetail, id, err); claimed {
		return m, tea.Batch(armed, cmd, owed)
	}
	if err != nil && held.Loaded {
		return m, tea.Batch(armed,
			m.toasts.Show(comp.ToastError, "Could not refresh #"+strconv.Itoa(m.detail.PullRequest().Number)))
	}
	return m, tea.Batch(armed, owed)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		if m.screen == screenDetail {
			return m, m.detail.Init()
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case viewerFetchedMsg:
		m.store.ViewerApplied(msg.res)
		m.detail.SetViewer(m.store.Viewer())
		return m, nil

	case viewerFailedMsg:
		return m, nil

	case newerReleaseMsg:
		m.newerRelease = msg.tag
		return m, nil

	case sectionFetchedMsg:
		m.store.Applied(msg.index, msg.res)
		m.poller.stampSection(msg.index, len(m.store.Sections()), time.Now())
		return m.sectionSettled()

	case sectionFailedMsg:
		m.store.Failed(msg.index, msg.err)
		m.poller.stampSection(msg.index, len(m.store.Sections()), time.Now())
		return m.sectionSettled()

	case sectionPollFailedMsg:
		m.poller.stampSection(msg.index, len(m.store.Sections()), time.Now())
		if slices.Contains(m.refreshing, msg.index) {
			m.store.Failed(msg.index, msg.err)
			return m.sectionSettled()
		}
		m.store.PollFailed(msg.index)
		return m, nil

	case spinner.TickMsg:
		var listCmd, detailCmd tea.Cmd
		m.list, listCmd = m.list.Update(msg)
		m.detail, detailCmd = m.detail.Update(msg)
		spinCmd := m.refreshSpin.Advance(msg, m.refreshRunning())
		return m, tea.Batch(listCmd, detailCmd, spinCmd)

	case comp.ToastExpiredMsg:
		m.toasts.Expire(msg)
		return m, nil

	case detailFetchedMsg:
		probe := m.probeMergeability(msg.id, msg.res)

		m.store.DetailApplied(msg.id, msg.res)
		m.poller.stampDetail(msg.id, time.Now())
		model, cmd := m.detailSettled(msg.id, nil)
		return model, tea.Batch(cmd, probe)

	case detailFailedMsg:
		m.store.DetailFailed(msg.id, msg.err)
		m.poller.stampDetail(msg.id, time.Now())
		return m.detailSettled(msg.id, msg.err)

	case pulseFetchedMsg:
		moved := m.store.PulseApplied(msg.id, msg.res)
		m.poller.stampDetail(msg.id, time.Now())
		return m.pulseSettled(msg.id, moved)

	case pulseFailedMsg:
		m.store.PulseFailed(msg.id)
		m.poller.stampDetail(msg.id, time.Now())
		return m, nil

	case pageFailedMsg:
		m.store.DetailFailed(msg.id, msg.err)
		m.poller.stampDetail(msg.id, time.Now())
		m.poller.stampPageFailed(msg.id, time.Now())
		cmd, _ := m.claim(legDetail, msg.id, msg.err)
		return m, cmd

	case pollTickMsg:
		return m.poll(msg)

	case checksTickMsg:
		return m.pollChecks(msg)

	case filesFetchedMsg:
		m.store.FilesApplied(msg.id, msg.res)
		return m.filesSettled(msg.id, nil)

	case filesFailedMsg:
		m.store.FilesFailed(msg.id, msg.err)
		return m.filesSettled(msg.id, msg.err)

	case fileViewedMsg:
		m.store.FileViewApplied(msg.id, msg.key)
		if !m.showing(msg.id) {
			return m, nil
		}
		return m, m.detail.SetFiles(m.store.Files(msg.id))

	case fileViewFailedMsg:
		m.store.FileViewReverted(msg.id, msg.key)
		toast := m.toasts.Show(comp.ToastError, "Could not update "+msg.path+": "+msg.err.Error())
		if !m.showing(msg.id) {
			return m, toast
		}
		return m, tea.Batch(m.detail.SetFiles(m.store.Files(msg.id)), toast)

	case commitFilesFetchedMsg:
		m.store.CommitFilesApplied(msg.sha, msg.res)
		return m.commitFilesSettled(msg.sha, nil)

	case commitFilesFailedMsg:
		m.store.CommitFilesFailed(msg.sha, msg.err)
		return m.commitFilesSettled(msg.sha, msg.err)

	case jobFetchedMsg:
		m.store.JobApplied(msg.id, msg.job, msg.log)
		return m.jobSettled(msg.id, nil, false)

	case jobFailedMsg:
		hadLoaded := m.store.Job(msg.id).Loaded
		if msg.job.ID != 0 {
			m.store.JobLogFailed(msg.id, msg.job, msg.err)
		} else {
			m.store.JobFailed(msg.id, msg.err)
		}
		return m.jobSettled(msg.id, msg.err, hadLoaded)

	case prview.NeedFilesMsg:
		return m.needFiles(msg.ID)

	case prview.ToggleFileViewedMsg:
		return m.toggleFileViewed(msg)

	case prview.NeedCommitMsg:
		return m.needCommit(msg.SHA)

	case prview.NeedJobMsg:
		return m.needJob(msg.JobID, msg.Refresh)

	case prview.RerunCheckMsg:
		return m.rerunCheck(msg)

	case checkRerunMsg:
		return m.checkRerunLanded(msg)

	case checkRerunFailedMsg:
		return m.checkRerunFailed(msg)

	case prview.RerunRunMsg:
		return m.rerunRun(msg)

	case runRerunMsg:
		return m.runRerunLanded(msg)

	case runRerunFailedMsg:
		return m.runRerunFailed(msg)

	case prview.RefreshMsg:
		return m.refreshDetail(msg)

	case prview.PostCommentMsg:
		return m.postComment(msg)

	case prview.EditorFailedMsg:
		return m, m.toasts.Show(comp.ToastError, "Could not open an editor: "+msg.Err.Error())

	case commentPostedMsg:
		return m.commentLanded(msg)

	case commentFailedMsg:
		return m.commentFailed(msg)

	case prview.PostReplyMsg:
		return m.postReply(msg)

	case replyPostedMsg:
		return m.replyLanded(msg)

	case replyFailedMsg:
		return m.replyFailed(msg)

	case prview.EditCommentMsg:
		return m.editComment(msg)

	case commentEditedMsg:
		return m.editLanded(msg)

	case commentEditFailedMsg:
		return m.editFailed(msg)

	case prview.DeleteCommentMsg:
		return m.deleteComment(msg)

	case commentDeletedMsg:
		return m.deleteLanded(msg)

	case commentDeleteFailedMsg:
		return m.deleteFailed(msg)

	case prview.SetBodyMsg:
		return m.setBody(msg)

	case bodySetMsg:
		return m.bodyLanded(msg)

	case bodyFailedMsg:
		return m.bodyFailed(msg)

	case prview.NeedRepoMetaMsg:
		return m.needRepoMeta(msg.Repo)

	case repoMetaFetchedMsg:
		return m.repoMetaLanded(msg)

	case repoMetaFailedMsg:
		return m.repoMetaFailed(msg)

	case prview.SetLabelsMsg:
		return m.setLabels(msg)

	case labelsSetMsg:
		return m.labelsLanded(msg)

	case labelsFailedMsg:
		return m.labelsFailed(msg)

	case prview.SetReviewersMsg:
		return m.setReviewers(msg)

	case reviewersSetMsg:
		return m.reviewersLanded(msg)

	case reviewersFailedMsg:
		return m.reviewersFailed(msg)

	case prview.SetAssigneesMsg:
		return m.setAssignees(msg)

	case assigneesSetMsg:
		return m.assigneesLanded(msg)

	case assigneesFailedMsg:
		return m.assigneesFailed(msg)

	case prview.SetStateMsg:
		return m.setState(msg)

	case stateSetMsg:
		return m.stateLanded(msg)

	case stateFailedMsg:
		return m.stateFailed(msg)

	case prview.NeedBranchesMsg:
		return m.needBranches(msg)

	case branchesFetchedMsg:
		return m.branchesLanded(msg)

	case branchesFailedMsg:
		return m.branchesFailed(msg)

	case prview.SetBaseMsg:
		return m.setBase(msg)

	case baseSetMsg:
		return m.baseLanded(msg)

	case baseFailedMsg:
		return m.baseFailed(msg)

	case prview.MergeMsg:
		return m.merge(msg)

	case mergedMsg:
		return m.mergeLanded(msg)

	case mergeFailedMsg:
		return m.mergeFailed(msg)

	case refDeleteFailedMsg:
		return m.refDeleteFailed(msg)

	case mergeProbeMsg:
		return m.mergeProbe(msg)

	case prview.ResolveThreadMsg:
		return m.resolveThread(msg)

	case threadResolvedMsg:
		return m.resolveLanded(msg)

	case resolveFailedMsg:
		return m.resolveFailed(msg)

	case prview.ReactMsg:
		return m.react(msg)

	case reactedMsg:
		return m.reactionLanded(msg)

	case reactFailedMsg:
		return m.reactionFailed(msg)

	case prview.ThreadNotInDiffMsg:
		return m, m.toasts.Show(comp.ToastInfo, msg.Path+" is not in the diff")

	case prview.SplitTooNarrowMsg:
		return m, m.toasts.Show(comp.ToastInfo,
			"Side by side needs "+comp.Plural(msg.Short, "more column")+" in the pane")

	case tea.KeyboardEnhancementsMsg:
		m.chords = msg.SupportsKeyDisambiguation()
		m.detail.SetChords(m.chords)
		return m, nil

	case list.OpenMsg:
		return m.open(msg.PR)

	case list.RefreshMsg:
		return m.refresh()

	case list.CopyLinkMsg:
		return m, copyLinkCmd(msg.PR)

	case prview.CopyLinkMsg:
		return m, copyLinkCmd(msg.PR)

	case list.BrowseMsg:
		return m, browseCmd(msg.PR)

	case prview.BrowseMsg:
		return m, browseCmd(msg.PR)

	case linkCopiedMsg:
		return m.linkCopied(msg)

	case browseFailedMsg:
		return m, m.toasts.Show(comp.ToastError, "Could not open a browser: "+msg.err.Error())

	case prview.BackMsg:
		m.screen = screenList
		m.detailRefreshing = detailRefresh{}
		m.resize()
		return m, nil
	}

	return m.delegate(msg)
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, keys.Global.ForceQuit) {
		return m, tea.Quit
	}

	capturing := m.capturing()

	if m.width < minWidth || m.height < minHeight {
		switch {
		case msg.String() == "esc":
			return m.delegate(msg)
		case key.Matches(msg, keys.Global.Quit) && !capturing:
			return m, tea.Quit
		}
		return m, nil
	}

	if capturing {
		return m.delegate(msg)
	}

	switch {
	case key.Matches(msg, keys.Global.Quit):
		return m, tea.Quit

	case key.Matches(msg, keys.Global.Help):
		m.showHelp = !m.showHelp
		return m, nil
	}

	if m.showHelp {
		if msg.String() == "esc" {
			m.showHelp = false
		}
		return m, nil
	}

	return m.delegate(msg)
}

func (m Model) capturing() bool {
	switch m.screen {
	case screenDetail:
		return m.detail.Capturing()
	case screenList:
		return m.list.Capturing()
	}
	return false
}

func (m Model) delegate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.screen {
	case screenList:
		m.list, cmd = m.list.Update(msg)
	case screenDetail:
		wasChecks := m.detail.ShowsChecks()
		m.detail, cmd = m.detail.Update(msg)
		nowChecks := m.detail.ShowsChecks()
		if !wasChecks && nowChecks {
			var checks tea.Cmd
			m, checks = m.startChecks()
			cmd = tea.Batch(cmd, checks)
		}
	}
	return m, cmd
}

func (m *Model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	if m.width < minWidth || m.height < minHeight {
		return
	}

	body := max(0, m.height-statusBarHeight-m.noticeHeight())
	m.status = m.status.Size(m.width)
	m.help.SetWidth(m.width)

	switch m.screen {
	case screenList:
		m.list.SetSize(m.width, body)
	case screenDetail:
		m.detail.SetSize(m.width, body)
	}
}

func (m Model) noticeHeight() int {
	if m.notice == "" || m.height < 2 {
		return 0
	}
	return 1
}

func (m Model) screenView() string {
	if m.screen == screenDetail {
		return m.detail.View()
	}
	return m.list.View()
}

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.Cursor = m.cursor()

	v.BackgroundColor = m.theme.Background
	return v
}

func (m Model) cursor() *tea.Cursor {
	if m.width < minWidth || m.height < minHeight {
		return nil
	}
	if m.showHelp {
		return nil
	}

	var c *tea.Cursor
	if m.screen == screenDetail {
		c = m.detail.Cursor()
	} else {
		c = m.list.Cursor()
	}
	return comp.Offset(c, 0, m.noticeHeight())
}

func (m Model) render() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	if m.width < minWidth || m.height < minHeight {
		return m.tooSmall()
	}

	parts := make([]string, 0, 3)
	if m.noticeHeight() > 0 {
		parts = append(parts, m.status.Render(m.noticeLine(), ""))
	}
	if body := m.screenView(); body != "" {
		parts = append(parts, body)
	}
	if message := m.statusMessage(); message != "" {
		parts = append(parts, m.status.RenderMessage(m.statusHints(m.status.MessageRoom(message)), message))
	} else {
		parts = append(parts, m.status.Render(m.statusHints(m.status.Room()), m.statusReadout()))
	}

	frame := strings.Join(parts, "\n")
	if !m.showHelp {
		return frame
	}
	return comp.Over(frame, comp.Modal(m.theme, "Keys", m.helpBody()), m.width, m.height)
}

func (m Model) noticeLine() string {
	return lipgloss.NewStyle().Foreground(m.theme.Warning).Render(m.notice)
}

func (m Model) statusHints(room int) string {
	if m.screen != screenDetail {
		return m.shedHints(m.list.ShortHelp(), room)
	}
	return m.shedHints(m.detail.ShortHelp(), room)
}

// Measures the rendered line, since only the help bubble knows what a hint costs.
func (m Model) shedHints(hints []key.Binding, room int) string {
	h := m.help
	h.SetWidth(0)
	for len(hints) > 1 {
		if line := h.ShortHelpView(hints); lipgloss.Width(line) <= room {
			return line
		}
		last := len(hints) - 1
		hints = append(hints[:last-1:last-1], hints[last])
	}
	return h.ShortHelpView(hints)
}

func (m Model) statusMessage() string {
	if !m.toasts.Empty() {
		return m.toasts.Render(m.theme)
	}
	if m.refreshRunning() {
		return m.refreshSpin.RenderAccent("Refreshing")
	}
	return ""
}

func (m Model) refreshRunning() bool {
	return m.detailRefreshing.running() || len(m.refreshing) > 0
}

func (m Model) statusReadout() string {
	if rate := m.store.Rate(); rate.Limit > 0 {
		if budget := m.status.Budget(rate.Remaining); budget != "" {
			return budget
		}
	}
	if m.screen == screenDetail {
		if readout := m.detail.Readout(); readout != "" {
			return lipgloss.NewStyle().Foreground(m.theme.MutedOrSubtle()).Render(readout)
		}
	}
	return m.releaseNotice()
}

func (m Model) helpBody() string {
	groups := m.list.Keys().FullHelp()
	if m.screen == screenDetail {
		groups = m.detail.Keys().FullHelp()
	}

	body := m.help.FullHelpView(refitHelp(groups, m.width-modalChrome))

	room := m.height - statusBarHeight - m.noticeHeight() - 2
	if strings.Count(body, "\n")+1 <= room {
		return body
	}

	note := lipgloss.NewStyle().Foreground(m.theme.Warning).
		Render("… more keys than this frame can show")
	return strings.Join(append(strings.Split(body, "\n")[:max(0, room-1)], note), "\n")
}

const modalChrome = 4

// Capped so the list reads down a column rather than across a wide terminal.
const helpColumns = 3

// The help bubble never wraps, so an overwide set would be sheared by the overlay rather than reflowed.
func refitHelp(groups [][]key.Binding, width int) [][]key.Binding {
	var flat []key.Binding
	widestKey, widestDesc := 0, 0
	for _, group := range groups {
		for _, b := range group {
			flat = append(flat, b)
			widestKey = max(widestKey, len(b.Help().Key))
			widestDesc = max(widestDesc, len(b.Help().Desc))
		}
	}
	if len(flat) == 0 {
		return groups
	}

	const columnGap = 4
	columns := max(1, min(helpColumns, width/(widestKey+widestDesc+1+columnGap)))
	if columns >= len(groups) {
		return groups
	}

	rows := (len(flat) + columns - 1) / columns
	out := make([][]key.Binding, 0, columns)
	for i := 0; i < len(flat); i += rows {
		out = append(out, flat[i:min(i+rows, len(flat))])
	}
	return out
}

func helpStyles(th theme.Theme) help.Styles {
	key := lipgloss.NewStyle().Foreground(th.Accent)
	desc := lipgloss.NewStyle().Foreground(th.MutedOrSubtle())
	sep := lipgloss.NewStyle().Foreground(th.BorderMutedOrSubtle())

	return help.Styles{
		Ellipsis:       sep,
		ShortKey:       key,
		ShortDesc:      desc,
		ShortSeparator: sep,
		FullKey:        key,
		FullDesc:       desc,
		FullSeparator:  sep,
	}
}

func refreshSummary(sections []store.Section, started []int) (comp.ToastKind, string) {
	failed := 0
	for _, i := range started {
		if sections[i].Status == store.StatusFailed {
			failed++
		}
	}

	switch {
	case failed == 0:
		return comp.ToastSuccess, "Refreshed " + comp.Plural(len(started), "section")
	case failed == len(started):
		return comp.ToastError, "Refresh failed"
	default:
		return comp.ToastError, "Refreshed " + comp.Plural(len(started)-failed, "section") +
			", " + strconv.Itoa(failed) + " failed"
	}
}
