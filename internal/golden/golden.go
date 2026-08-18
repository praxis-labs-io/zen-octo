// Package golden compares test output against a recorded file, and rewrites the
// file instead when -update is passed. Test-only: it never reaches the binary.
package golden

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// Registered here rather than per suite, so `make golden` regenerates every
// golden under one name. It exists only in a binary linking this package.
var update = flag.Bool("update", false, "regenerate the golden files")

// The directory the test binary started in. A test that chdirs would otherwise
// resolve testdata against wherever it moved to.
var root = startDir()

func startDir() string {
	wd, err := os.Getwd()
	if err != nil {
		panic("golden: reading the working directory: " + err.Error())
	}
	return wd
}

// Compare checks got against testdata/<name>.golden beside the calling package,
// failing when they differ, or writing it there under -update.
func Compare(t *testing.T, name string, got []byte) {
	t.Helper()

	dir := filepath.Join(root, "testdata")
	path := filepath.Join(dir, name+".golden")
	if *update {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("creating testdata: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v (run: make golden)", path, err)
	}
	if string(got) != string(want) {
		t.Errorf("%s changed.\n got %s\nwant %s", name, got, want)
	}
}
