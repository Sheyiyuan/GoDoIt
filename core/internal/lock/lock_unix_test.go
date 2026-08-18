//go:build linux || darwin

package lock

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireRespondsToContextCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")
	first, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	second, err := Acquire(ctx, path)
	if second != nil {
		_ = second.Close()
		t.Fatal("second lock acquisition unexpectedly succeeded")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
}
