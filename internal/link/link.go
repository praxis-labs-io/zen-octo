// Package link hands a URL to the clipboard or a browser.
package link

import (
	"io"

	"github.com/atotto/clipboard"
	"github.com/cli/go-gh/v2/pkg/browser"
)

// Copy writes s to the system clipboard. It fails where the platform has no clipboard tool.
func Copy(s string) error { return clipboard.WriteAll(s) }

// Browse opens u the way gh does: GH_BROWSER, gh's config, BROWSER, then the platform
// default. The browser's output is discarded.
func Browse(u string) error { return browser.New("", io.Discard, io.Discard).Browse(u) }
