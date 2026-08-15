// Package link puts a URL somewhere outside the terminal: on the clipboard, or
// in front of the reader in a browser.
package link

import (
	"io"

	"github.com/atotto/clipboard"
	"github.com/cli/go-gh/v2/pkg/browser"
)

// Copy writes s to the system clipboard through whatever the platform runs. It
// fails where there is no clipboard tool, which is the case OSC52 answers.
func Copy(s string) error { return clipboard.WriteAll(s) }

// Browse opens u the way gh does: GH_BROWSER, then gh's config, then BROWSER,
// then the platform default. Its streams go nowhere; the TUI owns stdout.
func Browse(u string) error { return browser.New("", io.Discard, io.Discard).Browse(u) }
