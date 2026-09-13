package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/praxis-labs-io/zen-octo/internal/config"
	"github.com/praxis-labs-io/zen-octo/internal/tui/app"
	"github.com/praxis-labs-io/zen-octo/internal/update"
)

const releaseNotice = "v9.9.9 is available. Run zen-octo update."

func releaseAnswering(result update.Result, err error) app.ReleaseCheck {
	return func(context.Context) (update.Result, error) { return result, err }
}

var newerRelease app.ReleaseCheck = releaseAnswering(update.Result{Latest: "v9.9.9", Available: true}, nil)

func launched(t *testing.T, cfg *config.Config, client *fakeSearcher, check app.ReleaseCheck) tea.Model {
	t.Helper()
	return drive(t, app.New(cfg, client, testSurface, check), tea.WindowSizeMsg{Width: 160, Height: 40})
}

func TestANewerReleaseIsNamedOnTheListBar(t *testing.T) {
	frame := render(t, launched(t, testConfig(), &fakeSearcher{prs: samplePRs()}, newerRelease))

	if bar := lastLine(frame); !strings.Contains(bar, releaseNotice) {
		t.Fatalf("status bar = %q, want %q on it", strings.TrimSpace(bar), releaseNotice)
	}

	lines := strings.Split(frame, "\n")
	raw := lines[len(lines)-1]
	if !strings.Contains(raw, fgSeq(testTheme.Subtle)) || strings.Contains(raw, fgSeq(testTheme.Error)) {
		t.Error("the release notice is not drawn as quiet text")
	}
}

func TestTheBarSaysNothingWithoutANewerRelease(t *testing.T) {
	off := false
	quiet := testConfig()
	quiet.UpdateCheck = &off

	tests := []struct {
		name  string
		cfg   *config.Config
		check app.ReleaseCheck
	}{
		{name: "already current", cfg: testConfig(), check: releaseAnswering(update.Result{Latest: "v9.9.9"}, nil)},
		{name: "a failed lookup", cfg: testConfig(), check: releaseAnswering(update.Result{}, errors.New("the latest release lookup answered 403 Forbidden"))},
		{name: "turned off in config", cfg: quiet, check: newerRelease},
		{name: "no checker", cfg: testConfig(), check: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := render(t, launched(t, tt.cfg, &fakeSearcher{prs: samplePRs()}, tt.check))
			if strings.Contains(stripANSI(frame), "is available") {
				t.Errorf("frame carries a release notice:\n%s", stripANSI(frame))
			}
		})
	}
}

func TestTheReadoutOutranksTheReleaseNotice(t *testing.T) {
	client := &fakeSearcher{prs: samplePRs()}
	client.serveDetail("PR_412", "Caps the backoff at 30s.")

	m := press(launched(t, testConfig(), client, newerRelease), "enter")

	bar := lastLine(render(t, m))
	if !strings.Contains(bar, "@drucial") {
		t.Errorf("status bar = %q, want who opened the pull request kept", strings.TrimSpace(bar))
	}
	if strings.Contains(bar, "is available") {
		t.Errorf("status bar = %q, want the release notice to give way to the readout", strings.TrimSpace(bar))
	}

	if bar := lastLine(render(t, press(m, "esc"))); !strings.Contains(bar, releaseNotice) {
		t.Errorf("list bar = %q, want the notice back once the pull request closes", strings.TrimSpace(bar))
	}
}
