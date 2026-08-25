//go:build linux || darwin

package store

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRawCurrentPointer(t *testing.T, root, target string) {
	t.Helper()
	if err := os.Symlink(target, filepath.Join(root, "current")); err != nil {
		t.Fatal(err)
	}
}
