//go:build windows

package store

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRawCurrentPointer(t *testing.T, root, target string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "current"), []byte(target+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
