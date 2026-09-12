package app_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/praxis-labs-io/zen-octo/internal/tui/app"
	"github.com/praxis-labs-io/zen-octo/internal/tui/prview"
)

const prURL = "https://github.com/acme/rocket/pull/412"

func links(t *testing.T, copyErr, browseErr error) (copied, opened *string) {
	t.Helper()

	var gotCopy, gotBrowse string
	app.StubLinks(t,
		func(s string) error { gotCopy = s; return copyErr },
		func(s string) error { gotBrowse = s; return browseErr },
	)
	return &gotCopy, &gotBrowse
}

func TestYCopiesTheSelectedPullRequestsLink(t *testing.T) {
	copied, _ := links(t, nil, nil)

	m := press(loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40), "y")

	if *copied != prURL {
		t.Errorf("copied %q, want %q", *copied, prURL)
	}
	if bar := lastLine(render(t, m)); !strings.Contains(bar, "Copied the link to #412") {
		t.Errorf("status bar = %q, want the copy reported", strings.TrimSpace(bar))
	}
}

func TestAFailedNativeCopyStillReportsTheLink(t *testing.T) {
	links(t, errors.New("no pbcopy here"), nil)

	m := press(loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40), "y")

	bar := lastLine(render(t, m))
	if !strings.Contains(bar, "Copied the link to #412") {
		t.Errorf("status bar = %q, want the copy reported", strings.TrimSpace(bar))
	}
	if strings.Contains(bar, "no pbcopy here") {
		t.Errorf("status bar = %q, want no error beside a copy that worked", strings.TrimSpace(bar))
	}
}

func TestOOpensTheSelectedPullRequestAndSaysNothing(t *testing.T) {
	_, opened := links(t, nil, nil)

	m := press(loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40), "O")

	if *opened != prURL {
		t.Errorf("opened %q, want %q", *opened, prURL)
	}
	if bar := lastLine(render(t, m)); strings.Contains(bar, "Copied") || strings.Contains(bar, "Could not") {
		t.Errorf("status bar = %q, want nothing said about a browser that opened", strings.TrimSpace(bar))
	}
}

func TestABrowserThatWillNotLaunchIsReported(t *testing.T) {
	links(t, nil, errors.New("exec: \"firefox\": not found"))

	m := press(loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40), "O")

	if bar := lastLine(render(t, m)); !strings.Contains(bar, "Could not open a browser") {
		t.Errorf("status bar = %q, want the failure reported", strings.TrimSpace(bar))
	}
}

func TestTheDetailScreensLinkKeysReachTheSameHandlers(t *testing.T) {
	copied, opened := links(t, nil, nil)
	pr := samplePRs()[0]

	m := loaded(t, &fakeSearcher{prs: samplePRs()}, 120, 40)
	m = settle(m, prview.CopyLinkMsg{PR: pr}, prview.BrowseMsg{PR: pr})

	if *copied != prURL {
		t.Errorf("copied %q, want %q", *copied, prURL)
	}
	if *opened != prURL {
		t.Errorf("opened %q, want %q", *opened, prURL)
	}
	if bar := lastLine(render(t, m)); !strings.Contains(bar, "Copied the link to #412") {
		t.Errorf("status bar = %q, want the copy reported", strings.TrimSpace(bar))
	}
}
