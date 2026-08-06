package gh

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// filesPage is how many files one request asks for, which is the most the REST
// API will return.
const filesPage = 100

// fileNode is GitHub's REST shape for one changed file. Patch is absent on a
// binary file and on a diff too large to return.
type fileNode struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename"`
	Status           string `json:"status"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Patch            string `json:"patch"`
}

// PullRequestFiles fetches the diff. GraphQL's files connection carries the
// paths and the churn but no patch, so this is the one call in the package that
// goes over REST.
//
// changedFiles is what the pull request says it touched. The response is capped
// at one page, and without the number the caller already holds there is no way
// to say how many files went unfetched.
func (c *Client) PullRequestFiles(ctx context.Context, repo string, number, changedFiles int) (FilesResult, error) {
	if !strings.Contains(repo, "/") {
		return FilesResult{}, fmt.Errorf("fetching files (%s#%d): %q is not owner/name", repo, number, repo)
	}

	path := fmt.Sprintf("repos/%s/pulls/%d/files?per_page=%d", repo, number, filesPage)

	var nodes []fileNode
	if err := c.rest.DoWithContext(ctx, http.MethodGet, path, nil, &nodes); err != nil {
		return FilesResult{}, fmt.Errorf("fetching files (%s#%d): %w", repo, number, classify(err))
	}

	return FilesResult{
		Files:     changed(nodes),
		MoreFiles: max(0, changedFiles-len(nodes)),
	}, nil
}

// CommitFiles fetches one commit's diff. The commit endpoint answers with the
// same file nodes the pull request one does, so everything below the request is
// shared.
//
// It reports overflow as a flag rather than a count: the response carries no
// changed-file total, so a full page is all there is to go on.
func (c *Client) CommitFiles(ctx context.Context, repo, sha string) (FilesResult, error) {
	if !strings.Contains(repo, "/") {
		return FilesResult{}, fmt.Errorf("fetching commit (%s@%s): %q is not owner/name", repo, sha, repo)
	}

	path := fmt.Sprintf("repos/%s/commits/%s?per_page=%d", repo, sha, filesPage)

	var resp struct {
		Files []fileNode `json:"files"`
	}
	if err := c.rest.DoWithContext(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return FilesResult{}, fmt.Errorf("fetching commit (%s@%s): %w", repo, sha, classify(err))
	}

	return FilesResult{
		Files:     changed(resp.Files),
		Truncated: len(resp.Files) >= filesPage,
	}, nil
}

// changed turns the REST file nodes into the domain type.
func changed(nodes []fileNode) []ChangedFile {
	out := make([]ChangedFile, 0, len(nodes))
	for _, n := range nodes {
		file := ChangedFile{
			Path:         n.Filename,
			PreviousPath: n.PreviousFilename,
			Status:       FileStatus(n.Status),
			Additions:    n.Additions,
			Deletions:    n.Deletions,
		}
		if n.Patch == "" {
			file.Omitted = omission(file)
		} else {
			file.Hunks = hunks(n.Patch)
		}
		out = append(out, file)
	}
	return out
}

// omission says why a file arrived without a patch. GitHub does not tell us
// which reason applies, but the churn does: a file that changed by no lines was
// moved or had its mode set, and one that changed by many is binary or too big
// for the API to render.
func omission(f ChangedFile) string {
	if f.Additions == 0 && f.Deletions == 0 {
		if f.Status == FileRenamed || f.Status == FileCopied {
			return "no changes to the contents"
		}
		return "no line changes"
	}
	return "binary, or too large for GitHub to return a diff"
}

// hunkHeader is the @@ line. The counts are optional: a one-line range is
// written without one, and a heading may or may not follow the closing @@.
var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// hunks parses a patch into its blocks. Anything before the first @@ is
// dropped: GitHub's patch starts at one, and a stray leading line would other
// wise land in a hunk it does not belong to.
func hunks(patch string) []Hunk {
	var out []Hunk
	var oldNo, newNo int

	// A trailing newline would otherwise split into an empty final line and
	// land in the last hunk as a blank one the file does not have.
	for _, line := range strings.Split(strings.TrimSuffix(patch, "\n"), "\n") {
		if m := hunkHeader.FindStringSubmatch(line); m != nil {
			oldNo, newNo = atoi(m[1]), atoi(m[3])
			out = append(out, Hunk{Header: line})
			continue
		}
		if len(out) == 0 {
			continue
		}

		h := &out[len(out)-1]
		switch {
		case strings.HasPrefix(line, "+"):
			h.Lines = append(h.Lines, DiffLine{Kind: DiffAdded, New: newNo, Content: line[1:]})
			newNo++
		case strings.HasPrefix(line, "-"):
			h.Lines = append(h.Lines, DiffLine{Kind: DiffRemoved, Old: oldNo, Content: line[1:]})
			oldNo++
		case strings.HasPrefix(line, `\`):
			// "\ No newline at end of file" annotates the line above it and
			// occupies no line number of its own.
		default:
			// A blank context line can arrive as the empty string rather than a
			// lone space, so the marker is cut as a prefix rather than by index.
			h.Lines = append(h.Lines, DiffLine{Kind: DiffContext, Old: oldNo, New: newNo, Content: strings.TrimPrefix(line, " ")})
			oldNo++
			newNo++
		}
	}
	return out
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
