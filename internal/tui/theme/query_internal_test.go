package theme

import (
	"bytes"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// reply drives the decode half of Query over a canned terminal answer, which is
// everything but the raw-mode dance.
func reply(t *testing.T, answer string) Surface {
	t.Helper()

	var s Surface
	read(strings.NewReader(answer), &bytes.Buffer{}, "", func(seq string, pa *ansi.Parser) bool {
		switch {
		case ansi.HasOscPrefix(seq):
			switch pa.Command() {
			case 10:
				s.Foreground = oscColor(pa)
			case 11:
				s.Background = oscColor(pa)
			}
		case ansi.HasCsiPrefix(seq):
			if pa.Command() == ansi.Command('?', 0, 'c') {
				return false
			}
		}
		return true
	})
	return s
}

func hexOf(t *testing.T, c color.Color) string {
	t.Helper()
	if c == nil {
		return "nil"
	}
	r, g, b, _ := c.RGBA()
	return string([]byte{'#'}) + hex2(byte(r>>8)) + hex2(byte(g>>8)) + hex2(byte(b>>8))
}

func hex2(b byte) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[b>>4], digits[b&0xf]})
}

func TestQueryReadsBothReports(t *testing.T) {
	got := reply(t, "\x1b]10;rgb:e0e0/dede/f4f4\x1b\\\x1b]11;rgb:2323/2121/3636\x1b\\\x1b[?62;c")

	if want := "#e0def4"; hexOf(t, got.Foreground) != want {
		t.Errorf("Foreground = %s, want %s", hexOf(t, got.Foreground), want)
	}
	if want := "#232136"; hexOf(t, got.Background) != want {
		t.Errorf("Background = %s, want %s", hexOf(t, got.Background), want)
	}
}

// Terminals answer with components of varying width, and a short one scales up
// rather than being read at face value.
func TestQueryScalesShortComponents(t *testing.T) {
	got := reply(t, "\x1b]11;rgb:1c/1c/1c\x1b\\\x1b[?62;c")
	if want := "#1c1c1c"; hexOf(t, got.Background) != want {
		t.Errorf("Background = %s, want %s", hexOf(t, got.Background), want)
	}
}

// One of the two answering is the common case on a terminal that supports only
// half of it, and the half that came back has to survive.
func TestQueryTakesWhicheverAnswered(t *testing.T) {
	got := reply(t, "\x1b]11;rgb:2323/2121/3636\x1b\\\x1b[?62;c")

	if got.Foreground != nil {
		t.Errorf("Foreground = %v, want nil when nothing reported one", got.Foreground)
	}
	if got.Background == nil {
		t.Error("Background is nil, want the report that did arrive")
	}
}

// The device attributes end the read. Reading only until the colors parsed would
// leave them in the buffer, and the terminal echoes them the moment raw mode
// ends, before anything has been drawn.
func TestTheDeviceAttributesEndTheRead(t *testing.T) {
	// Nothing follows the attributes here: a read that ran past them would block
	// on a reader with nothing left and be cancelled by the timeout instead.
	done := make(chan Surface, 1)
	go func() { done <- reply(t, "\x1b]11;rgb:2323/2121/3636\x1b\\\x1b[?62;c") }()

	select {
	case got := <-done:
		if got.Background == nil {
			t.Error("Background is nil, want the reply read before the attributes stopped it")
		}
	case <-time.After(queryTimeout / 2):
		t.Fatal("the read ran past the device attributes")
	}
}

// A terminal that answers nothing must not hang the launch, and must not leave a
// reader parked on the tty eating the first key pressed.
func TestASilentTerminalGivesUpAndReportsNothing(t *testing.T) {
	got := reply(t, "")
	if got.Background != nil || got.Foreground != nil {
		t.Errorf("Surface = %+v, want both nil when nothing answered", got)
	}
}

// A reply arriving in pieces is the ordinary case over a slow link: the decoder
// carries state across reads, so a sequence split mid-way still parses.
func TestASplitReplyStillParses(t *testing.T) {
	got := reply(t, "\x1b]11;rgb:23"+"23/2121/3636\x1b\\"+"\x1b[?62;c")
	if want := "#232136"; hexOf(t, got.Background) != want {
		t.Errorf("Background = %s, want %s", hexOf(t, got.Background), want)
	}
}
