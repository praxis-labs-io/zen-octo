package theme

import (
	"bytes"
	"image/color"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func reply(t *testing.T, answer string) Surface {
	t.Helper()
	return collect(strings.NewReader(answer), queryTimeout)
}

func collect(in io.Reader, timeout time.Duration) Surface {
	var s Surface
	read(in, &bytes.Buffer{}, "", timeout, s.take)
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

func TestQueryScalesShortComponents(t *testing.T) {
	got := reply(t, "\x1b]11;rgb:1c/1c/1c\x1b\\\x1b[?62;c")
	if want := "#1c1c1c"; hexOf(t, got.Background) != want {
		t.Errorf("Background = %s, want %s", hexOf(t, got.Background), want)
	}
}

func TestQueryTakesWhicheverAnswered(t *testing.T) {
	got := reply(t, "\x1b]11;rgb:2323/2121/3636\x1b\\\x1b[?62;c")

	if got.Foreground != nil {
		t.Errorf("Foreground = %v, want nil when nothing reported one", got.Foreground)
	}
	if got.Background == nil {
		t.Error("Background is nil, want the report that did arrive")
	}
}

// Nothing follows the attributes, so a read past them blocks until the timeout.
func TestTheDeviceAttributesEndTheRead(t *testing.T) {
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

// An os.Pipe, not any blocking reader: the cancel reader interrupts a file descriptor, not an arbitrary Read.
func TestASilentTerminalGivesUpAndReportsNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("on Windows the cancel reader interrupts only a console's stdin, and a pipe gets a fallback Cancel can't unblock")
	}

	silent, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("opening a pipe: %v", err)
	}
	defer silent.Close() //nolint:errcheck
	defer w.Close()      //nolint:errcheck

	done := make(chan Surface, 1)
	go func() { done <- collect(silent, 20*time.Millisecond) }()

	select {
	case got := <-done:
		if got.Background != nil || got.Foreground != nil {
			t.Errorf("Surface = %+v, want both nil when nothing answered", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the read never gave up, so a silent terminal hangs the launch")
	}
}

func TestASplitReplyStillParses(t *testing.T) {
	full := "\x1b]11;rgb:2323/2121/3636\x1b\\\x1b[?62;c"

	got := collect(&chunked{parts: []string{full[:12], full[12:]}}, queryTimeout)
	if want := "#232136"; hexOf(t, got.Background) != want {
		t.Errorf("Background = %s, want %s", hexOf(t, got.Background), want)
	}
}

// strings.Reader hands its whole answer over in one Read, which never exercises the carry.
type chunked struct{ parts []string }

func (c *chunked) Read(p []byte) (int, error) {
	if len(c.parts) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.parts[0])
	c.parts = c.parts[1:]
	return n, nil
}

func TestQueryReadsThePaletteSlots(t *testing.T) {
	got := reply(t, "\x1b]4;1;rgb:cccc/2424/1d1d\x1b\\\x1b]4;2;rgb:9898/9797/1a1a\x1b\\\x1b[?62;c")

	if want := "#cc241d"; hexOf(t, got.Red) != want {
		t.Errorf("Red = %s, want %s", hexOf(t, got.Red), want)
	}
	if want := "#98971a"; hexOf(t, got.Green) != want {
		t.Errorf("Green = %s, want %s", hexOf(t, got.Green), want)
	}
}

func TestQueryIgnoresPaletteSlotsItDidNotAskFor(t *testing.T) {
	got := reply(t, "\x1b]4;4;rgb:0000/0000/ffff\x1b\\\x1b[?62;c")

	if got.Red != nil || got.Green != nil {
		t.Errorf("Red = %v and Green = %v, want an unasked-for slot to land nowhere", got.Red, got.Green)
	}
}

func TestQueryTakesTheSurfaceWithoutThePalette(t *testing.T) {
	got := reply(t, "\x1b]11;rgb:2323/2121/3636\x1b\\\x1b[?62;c")

	if got.Background == nil {
		t.Error("Background is nil, want the report that did arrive")
	}
	if got.Red != nil || got.Green != nil {
		t.Errorf("Red = %v and Green = %v, want nil when the palette went unanswered", got.Red, got.Green)
	}
}
