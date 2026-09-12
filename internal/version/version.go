// Package version carries build metadata stamped in at link time.
package version

// Version is set with -ldflags by .github/workflows/release.yml; a plain go build reports "dev".
var Version = "dev"
