package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/praxis-labs-io/zen-octo/internal/tui/app"
)

func TestTheMockupPulseKeepsTheStateTheFetchShowed(t *testing.T) {
	m := drive(t, app.New(testConfig(), app.Mock{}, testSurface, nil), tea.WindowSizeMsg{Width: 160, Height: 44})
	m = beat(press(m, "enter"), app.PollIdle+time.Second)

	header, _, _ := strings.Cut(ansi.Strip(render(t, m)), "\n")
	for _, want := range []string{"Open", "approved"} {
		if !strings.Contains(header, want) {
			t.Errorf("after a pulse the header lost %q:\n%s", want, header)
		}
	}
}

func TestTheMockupPulseAgreesWithTheFetch(t *testing.T) {
	const id = "PR_webhook-replay"
	fetched, err := app.Mock{}.PullRequest(context.Background(), id, "")
	if err != nil {
		t.Fatal(err)
	}
	pulsed, err := app.Mock{}.Pulse(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}

	if !pulsed.Pulse.UpdatedAt.Equal(fetched.Detail.UpdatedAt) {
		t.Errorf("pulse updatedAt %v, fetch %v, so every pulse marks the timeline stale", pulsed.Pulse.UpdatedAt, fetched.Detail.UpdatedAt)
	}
}
