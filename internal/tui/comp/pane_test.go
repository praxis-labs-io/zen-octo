package comp_test

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/praxis-labs-io/zen-octo/internal/tui/comp"
)

func pane() comp.Pane { return comp.NewPane(testTheme) }

func fgSeq(c color.Color) string { return sgrParams(lipgloss.NewStyle().Foreground(c)) }

func TestPaneReportsTheSizeLeftForContent(t *testing.T) {
	p := pane().Size(40, 10)

	if got := p.InnerWidth(); got != 38 {
		t.Errorf("InnerWidth() = %d, want 38", got)
	}
	if got := p.InnerHeight(); got != 8 {
		t.Errorf("InnerHeight() = %d, want 8", got)
	}
}

func TestPaneNeverExceedsItsSize(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		content       string
	}{
		{name: "content shorter than the pane", width: 40, height: 8, content: "one\ntwo"},
		{name: "content taller than the pane", width: 40, height: 5, content: strings.Repeat("row\n", 40)},
		{name: "content wider than the pane", width: 20, height: 4, content: strings.Repeat("wide ", 40)},
		{name: "no content at all", width: 30, height: 6, content: ""},
		{name: "no room for a body", width: 30, height: 2, content: "one\ntwo"},
		{name: "one line of body", width: 30, height: 3, content: "one\ntwo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pane().Size(tt.width, tt.height).Render(tt.content)

			lines := strings.Split(got, "\n")
			if len(lines) != tt.height {
				t.Errorf("rendered %d lines, want %d", len(lines), tt.height)
			}
			for i, line := range lines {
				if w := lipgloss.Width(line); w != tt.width {
					t.Errorf("line %d is %d cells wide, want %d", i, w, tt.width)
				}
			}
		})
	}
}

func TestPaneRendersTabsInTheTopBorder(t *testing.T) {
	tabs := []comp.Tab{
		{Label: "My PRs", Badge: "12"},
		{Label: "Needs Review", Badge: "3"},
	}

	top := strings.Split(pane().Size(60, 5).Tabs(tabs, 1).Render(""), "\n")[0]

	if !strings.Contains(top, "My PRs") || !strings.Contains(top, "Needs Review") {
		t.Errorf("top border = %q, want both tab labels", top)
	}
	if !strings.Contains(top, "12") || !strings.Contains(top, "3") {
		t.Errorf("top border = %q, want both badges", top)
	}
	if !strings.Contains(stripANSI(top), "My PRs 12 - Needs Review 3") {
		t.Errorf("top border = %q, want the tabs joined by a dash", stripANSI(top))
	}
}

func TestPaneBrightensOnlyTheActiveTab(t *testing.T) {
	tabs := []comp.Tab{{Label: "Conversation"}, {Label: "Commits"}}

	top := strings.Split(pane().Size(60, 5).Tabs(tabs, 0).Render(""), "\n")[0]

	active := fgSeq(testTheme.Accent) + "mConversation"
	idle := fgSeq(testTheme.Subtle) + "mCommits"
	if !strings.Contains(top, active) {
		t.Errorf("top border = %q, want Conversation in the accent color", top)
	}
	if !strings.Contains(top, idle) {
		t.Errorf("top border = %q, want Commits faint", top)
	}
}

func TestPaneIndexLeadsTheTopBorder(t *testing.T) {
	top := strings.Split(pane().Size(40, 5).Index(2).Title("Files").Render(""), "\n")[0]

	plain := stripANSI(top)
	if !strings.HasPrefix(plain, "╭─[2]─Files") {
		t.Errorf("top border = %q, want the index flush against the corner", plain)
	}
}

func TestPaneOmitsTheIndexWhenUnset(t *testing.T) {
	top := stripANSI(strings.Split(pane().Size(40, 5).Title("Files").Render(""), "\n")[0])

	if strings.Contains(top, "[") {
		t.Errorf("top border = %q, want no index bracket", top)
	}
	if !strings.HasPrefix(top, "╭─Files") {
		t.Errorf("top border = %q, want the title flush against the corner", top)
	}
}

func TestPaneFallsBackToTheTitleWhenThereAreNoTabs(t *testing.T) {
	top := strings.Split(pane().Size(40, 5).Title("Details").Render(""), "\n")[0]

	if !strings.Contains(top, "Details") {
		t.Errorf("top border = %q, want the title", top)
	}
}

func TestPaneTabsWinOverTheTitle(t *testing.T) {
	p := pane().Size(50, 5).Title("Details").Tabs([]comp.Tab{{Label: "Conversation"}}, 0)
	top := strings.Split(p.Render(""), "\n")[0]

	if strings.Contains(top, "Details") {
		t.Errorf("top border = %q, want tabs to replace the title", top)
	}
}

func TestPaneFooterSitsAtTheRightOfTheBottomBorder(t *testing.T) {
	lines := strings.Split(pane().Size(40, 5).Footer("4 of 12").Render(""), "\n")
	bottom := lines[len(lines)-1]

	if !strings.Contains(bottom, "4 of 12") {
		t.Fatalf("bottom border = %q, want the footer", bottom)
	}

	if plain := stripANSI(bottom); !strings.HasSuffix(plain, "4 of 12─╯") {
		t.Errorf("bottom border = %q, want the footer one rune in from the corner", plain)
	}
}

func TestPaneBorderColorFollowsFocus(t *testing.T) {
	focused := pane().Size(30, 4).Focus(true).Render("")
	idle := pane().Size(30, 4).Render("")

	if !strings.Contains(focused, fgSeq(testTheme.Accent)) {
		t.Error("focused pane does not use the accent color for its border")
	}
	if strings.Contains(idle, fgSeq(testTheme.Accent)) {
		t.Error("idle pane uses the focused border color")
	}
	if !strings.Contains(idle, fgSeq(testTheme.BorderSubtle)) {
		t.Error("idle pane does not use the idle border color")
	}
}

func TestPaneClipsRatherThanWrapsLongTabStrips(t *testing.T) {
	tabs := make([]comp.Tab, 12)
	for i := range tabs {
		tabs[i] = comp.Tab{Label: "A section with a long name", Badge: "99"}
	}

	got := pane().Size(50, 4).Tabs(tabs, 0).Render("")

	for i, line := range strings.Split(got, "\n") {
		if w := lipgloss.Width(line); w != 50 {
			t.Errorf("line %d is %d cells wide, want 50", i, w)
		}
	}
}

func TestPaneRendersNothingWhenTooSmallToFrame(t *testing.T) {
	if got := pane().Size(1, 1).Render("content"); got != "" {
		t.Errorf("Render() = %q, want empty when there is no room for a border", got)
	}
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

func TestAHeaderIsRuledOffFromTheContent(t *testing.T) {
	out := comp.NewPane(testTheme).
		Header("octobot commented").
		Size(30, 6).
		Render("Coverage held.")

	lines := strings.Split(stripANSI(out), "\n")
	if len(lines) != 6 {
		t.Fatalf("pane is %d lines, want 6", len(lines))
	}

	if !strings.Contains(lines[1], "octobot commented") {
		t.Errorf("line 1 = %q, want the heading", lines[1])
	}
	if !strings.HasPrefix(lines[2], "├") || !strings.HasSuffix(lines[2], "┤") {
		t.Errorf("line 2 = %q, want a rule joined to both sides", lines[2])
	}
	if !strings.Contains(lines[3], "Coverage held.") {
		t.Errorf("line 3 = %q, want the content under the rule", lines[3])
	}
}

func TestChromeCountsTheHeaderAndItsRule(t *testing.T) {
	plain := comp.NewPane(testTheme)
	headed := plain.Header("octobot commented")

	if plain.Chrome() != 2 {
		t.Errorf("Chrome() = %d with no header, want 2", plain.Chrome())
	}
	if headed.Chrome() != 4 {
		t.Errorf("Chrome() = %d with a header, want 4", headed.Chrome())
	}

	out := headed.Size(30, 1+headed.Chrome()).Render("Coverage held.")
	if got := strings.Count(out, "\n") + 1; got != 5 {
		t.Errorf("pane is %d lines, want 5", got)
	}
}

func TestAPaneTooShortForBothKeepsTheContent(t *testing.T) {
	out := comp.NewPane(testTheme).
		Header("octobot commented").
		Size(30, 3).
		Render("Coverage held.")

	if !strings.Contains(stripANSI(out), "Coverage held.") {
		t.Errorf("pane = %q, want the content rather than the heading", stripANSI(out))
	}
}

func TestPaneAboveIsWhereTheContentActuallyStarts(t *testing.T) {
	tests := []struct {
		name string
		pane comp.Pane
	}{
		{"headed", pane().Header(" heading").Size(40, 10)},
		{"bare", pane().Size(40, 10)},
		{"no room for the heading", pane().Header(" heading").Size(40, 4)},
		{"too narrow to draw", pane().Header(" heading").Size(1, 10)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := tt.pane.Render("first line")
			lines := strings.Split(out, "\n")

			at := tt.pane.Above()
			if out == "" {
				if at != 0 {
					t.Errorf("Above() = %d on a pane that draws nothing, want 0", at)
				}
				return
			}
			if at >= len(lines) {
				t.Fatalf("Above() = %d, and the pane is %d lines", at, len(lines))
			}
			if !strings.Contains(lines[at], "first line") {
				t.Errorf("Above() = %d, which is %q rather than the content", at, lines[at])
			}
		})
	}
}

func sgrParams(s lipgloss.Style) string {
	out := s.Render("x")
	end := strings.Index(out, "m")
	if end < 0 {
		return ""
	}
	return out[len("\x1b["):end]
}
