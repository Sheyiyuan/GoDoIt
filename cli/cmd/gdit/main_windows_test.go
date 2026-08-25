//go:build windows

package main

import (
	"strings"
	"testing"
)

func TestPathContainsDirIsCaseInsensitiveOnWindows(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("PATH", strings.ToUpper(directory))
	if !pathContainsDir(strings.ToLower(directory)) {
		t.Fatal("PATH membership must be case-insensitive on Windows")
	}
}
