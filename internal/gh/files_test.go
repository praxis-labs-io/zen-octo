package gh

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

// fakeREST answers a REST call with canned JSON or a canned error.
type fakeREST struct {
	body string
	err  error

	gotMethod string
	gotPath   string
}

func (f *fakeREST) DoWithContext(_ context.Context, method, path string, _ io.Reader, response any) error {
	f.gotMethod, f.gotPath = method, path
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.body), response)
}

const filesBody = `[
  {
    "filename": "internal/gh/files.go",
    "status": "added",
    "additions": 3,
    "deletions": 0,
    "patch": "@@ -0,0 +1,3 @@\n+package gh\n+\n+const filesPage = 100"
  },
  {
    "filename": "internal/tui/prview/files.go",
    "previous_filename": "internal/tui/prview/diff.go",
    "status": "renamed",
    "additions": 1,
    "deletions": 1,
    "patch": "@@ -14,5 +14,5 @@ func newViewport() viewport.Model {\n \tvp := viewport.New()\n-\tvp.SoftWrap = true\n+\tvp.SoftWrap = false\n \tvp.FillHeight = true\n \treturn vp"
  },
  {
    "filename": "docs/screenshot.png",
    "status": "modified",
    "additions": 0,
    "deletions": 0
  },
  {
    "filename": "assets/logo.bin",
    "status": "modified",
    "additions": 12,
    "deletions": 4
  }
]`

func fetchFiles(t *testing.T) FilesResult {
	t.Helper()
	res, err := newWithDoer(nil, &fakeREST{body: filesBody}).
		PullRequestFiles(context.Background(), "zen-octo/zen-octo", 17, 4)
	if err != nil {
		t.Fatalf("PullRequestFiles: %v", err)
	}
	return res
}

func TestFilesAskForOnePageOfTheRightPullRequest(t *testing.T) {
	rest := &fakeREST{body: filesBody}
	if _, err := newWithDoer(nil, rest).
		PullRequestFiles(context.Background(), "zen-octo/zen-octo", 17, 4); err != nil {
		t.Fatalf("PullRequestFiles: %v", err)
	}

	if rest.gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", rest.gotMethod)
	}
	want := "repos/zen-octo/zen-octo/pulls/17/files?per_page=100"
	if rest.gotPath != want {
		t.Errorf("path = %q, want %q", rest.gotPath, want)
	}
}

func TestFilesCarryTheirPathsAndChurn(t *testing.T) {
	res := fetchFiles(t)

	if len(res.Files) != 4 {
		t.Fatalf("files = %d, want 4", len(res.Files))
	}
	first := res.Files[0]
	if first.Path != "internal/gh/files.go" {
		t.Errorf("path = %q", first.Path)
	}
	if first.Status != FileAdded {
		t.Errorf("status = %q, want added", first.Status)
	}
	if first.Additions != 3 || first.Deletions != 0 {
		t.Errorf("churn = +%d −%d, want +3 −0", first.Additions, first.Deletions)
	}
}

func TestARenameCarriesThePathItCameFrom(t *testing.T) {
	renamed := fetchFiles(t).Files[1]

	if renamed.PreviousPath != "internal/tui/prview/diff.go" {
		t.Errorf("previous path = %q", renamed.PreviousPath)
	}
	if renamed.Status != FileRenamed {
		t.Errorf("status = %q, want renamed", renamed.Status)
	}
}

func TestAFileWithNoPatchSaysWhyRatherThanReadingAsUnchanged(t *testing.T) {
	files := fetchFiles(t).Files

	image := files[2]
	if len(image.Hunks) != 0 {
		t.Errorf("hunks = %d, want none", len(image.Hunks))
	}
	if image.Omitted != "no line changes" {
		t.Errorf("omitted = %q", image.Omitted)
	}

	binary := files[3]
	if !strings.Contains(binary.Omitted, "binary") {
		t.Errorf("omitted = %q, want it to name the binary case", binary.Omitted)
	}
}

func TestOverflowIsReportedAgainstWhatThePullRequestTouched(t *testing.T) {
	res, err := newWithDoer(nil, &fakeREST{body: filesBody}).
		PullRequestFiles(context.Background(), "zen-octo/zen-octo", 17, 130)
	if err != nil {
		t.Fatalf("PullRequestFiles: %v", err)
	}
	if res.MoreFiles != 126 {
		t.Errorf("MoreFiles = %d, want 126", res.MoreFiles)
	}
}

func TestAFullPageOfFilesReportsNoOverflow(t *testing.T) {
	if got := fetchFiles(t).MoreFiles; got != 0 {
		t.Errorf("MoreFiles = %d, want 0", got)
	}
}

// commitBody is the commit endpoint's answer: the same file nodes wrapped in an
// object rather than returned as a bare array.
const commitBody = `{
  "sha": "a3f91c2d5e",
  "files": [
    {
      "filename": "internal/gh/files.go",
      "status": "modified",
      "additions": 1,
      "deletions": 0,
      "patch": "@@ -1,2 +1,3 @@\n package gh\n+\n const filesPage = 100"
    }
  ]
}`

func TestACommitDiffAsksTheCommitEndpointAndParsesItsFiles(t *testing.T) {
	rest := &fakeREST{body: commitBody}
	res, err := newWithDoer(nil, rest).
		CommitFiles(context.Background(), "zen-octo/zen-octo", "a3f91c2d5e")
	if err != nil {
		t.Fatalf("CommitFiles: %v", err)
	}

	want := "repos/zen-octo/zen-octo/commits/a3f91c2d5e?per_page=100"
	if rest.gotMethod != http.MethodGet || rest.gotPath != want {
		t.Errorf("asked %s %q, want GET %q", rest.gotMethod, rest.gotPath, want)
	}

	if len(res.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(res.Files))
	}
	if got := res.Files[0].Path; got != "internal/gh/files.go" {
		t.Errorf("path = %q", got)
	}
	if len(res.Files[0].Hunks) != 1 {
		t.Errorf("hunks = %d, want the patch parsed", len(res.Files[0].Hunks))
	}
	if res.Truncated {
		t.Error("a single-file commit reads as truncated")
	}
}

// The commit endpoint carries no changed-file total, so a full page is the only
// sign GitHub is holding more.
func TestAFullPageOfCommitFilesReadsAsTruncated(t *testing.T) {
	nodes := make([]string, filesPage)
	for i := range nodes {
		nodes[i] = `{"filename": "a.go", "status": "modified", "additions": 1, "deletions": 0,
		  "patch": "@@ -1 +1 @@\n-a\n+b"}`
	}
	body := `{"files": [` + strings.Join(nodes, ",") + `]}`

	res, err := newWithDoer(nil, &fakeREST{body: body}).
		CommitFiles(context.Background(), "zen-octo/zen-octo", "a3f91c2")
	if err != nil {
		t.Fatalf("CommitFiles: %v", err)
	}
	if !res.Truncated {
		t.Error("a full page does not report that there is more")
	}
}

func TestACommitDiffForARepoWithoutAnOwnerIsRefusedBeforeTheRequest(t *testing.T) {
	rest := &fakeREST{body: commitBody}
	if _, err := newWithDoer(nil, rest).
		CommitFiles(context.Background(), "zen-octo", "a3f91c2"); err == nil {
		t.Fatal("want an error for a repo with no owner")
	}
	if rest.gotPath != "" {
		t.Errorf("requested %q, want no request at all", rest.gotPath)
	}
}

func TestARepoWithoutAnOwnerIsRefusedBeforeTheRequest(t *testing.T) {
	rest := &fakeREST{body: filesBody}
	_, err := newWithDoer(nil, rest).PullRequestFiles(context.Background(), "zen-octo", 17, 1)

	if err == nil {
		t.Fatal("want an error for a repo with no owner")
	}
	if rest.gotPath != "" {
		t.Errorf("requested %q, want no request at all", rest.gotPath)
	}
}

func TestAForbiddenFilesCallNamesTheScopeToAdd(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Oauth-Scopes", "gist, read:org")
	headers.Set("X-Accepted-Oauth-Scopes", "repo")

	rest := &fakeREST{err: &api.HTTPError{StatusCode: 403, Headers: headers}}
	_, err := newWithDoer(nil, rest).PullRequestFiles(context.Background(), "zen-octo/zen-octo", 17, 1)

	var scope *ScopeError
	if !errors.As(err, &scope) {
		t.Fatalf("err = %v, want a ScopeError", err)
	}
	if !strings.Contains(err.Error(), "gh auth refresh -s repo") {
		t.Errorf("err = %q, want the refresh command", err)
	}
}

func TestHunkParsing(t *testing.T) {
	tests := []struct {
		name  string
		patch string
		want  []DiffLine
	}{
		{
			name:  "an added file numbers only the new side",
			patch: "@@ -0,0 +1,2 @@\n+package gh\n+",
			want: []DiffLine{
				{Kind: DiffAdded, New: 1, Content: "package gh"},
				{Kind: DiffAdded, New: 2, Content: ""},
			},
		},
		{
			name:  "a removed line numbers only the old side",
			patch: "@@ -3,2 +3,1 @@\n-gone\n kept",
			want: []DiffLine{
				{Kind: DiffRemoved, Old: 3, Content: "gone"},
				{Kind: DiffContext, Old: 4, New: 3, Content: "kept"},
			},
		},
		{
			name:  "the two sides advance independently",
			patch: "@@ -10,3 +10,3 @@\n before\n-old\n+new\n after",
			want: []DiffLine{
				{Kind: DiffContext, Old: 10, New: 10, Content: "before"},
				{Kind: DiffRemoved, Old: 11, Content: "old"},
				{Kind: DiffAdded, New: 11, Content: "new"},
				{Kind: DiffContext, Old: 12, New: 12, Content: "after"},
			},
		},
		{
			name:  "a one-line range has no comma",
			patch: "@@ -7 +7 @@\n-a\n+b",
			want: []DiffLine{
				{Kind: DiffRemoved, Old: 7, Content: "a"},
				{Kind: DiffAdded, New: 7, Content: "b"},
			},
		},
		{
			name:  "no newline at end of file takes no line number",
			patch: "@@ -1,1 +1,1 @@\n-a\n\\ No newline at end of file\n+b",
			want: []DiffLine{
				{Kind: DiffRemoved, Old: 1, Content: "a"},
				{Kind: DiffAdded, New: 1, Content: "b"},
			},
		},
		{
			name:  "a blank context line arriving empty still counts",
			patch: "@@ -1,3 +1,3 @@\n a\n\n b",
			want: []DiffLine{
				{Kind: DiffContext, Old: 1, New: 1, Content: "a"},
				{Kind: DiffContext, Old: 2, New: 2, Content: ""},
				{Kind: DiffContext, Old: 3, New: 3, Content: "b"},
			},
		},
		{
			name:  "a trailing newline adds no line",
			patch: "@@ -1,1 +1,1 @@\n a\n",
			want: []DiffLine{
				{Kind: DiffContext, Old: 1, New: 1, Content: "a"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hunks(tt.patch)
			if len(got) != 1 {
				t.Fatalf("hunks = %d, want 1", len(got))
			}
			if len(got[0].Lines) != len(tt.want) {
				t.Fatalf("lines = %d, want %d: %+v", len(got[0].Lines), len(tt.want), got[0].Lines)
			}
			for i, line := range got[0].Lines {
				if line != tt.want[i] {
					t.Errorf("line %d = %+v, want %+v", i, line, tt.want[i])
				}
			}
		})
	}
}

func TestAMultiHunkPatchKeepsItsHeadings(t *testing.T) {
	patch := "@@ -1,1 +1,1 @@ func one() {\n-a\n@@ -20,1 +20,1 @@ func two() {\n+b"

	got := hunks(patch)
	if len(got) != 2 {
		t.Fatalf("hunks = %d, want 2", len(got))
	}
	if !strings.HasSuffix(got[0].Header, "func one() {") {
		t.Errorf("first header = %q", got[0].Header)
	}
	if got[1].Lines[0].New != 20 {
		t.Errorf("second hunk starts at %d, want 20", got[1].Lines[0].New)
	}
}

func TestAPatchWithNoHunkHeaderYieldsNothing(t *testing.T) {
	if got := hunks("just some text\nwith no marker"); len(got) != 0 {
		t.Errorf("hunks = %d, want none", len(got))
	}
}
