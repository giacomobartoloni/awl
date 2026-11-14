//go:build windows

package awl

import (
	"path/filepath"
	"testing"
	"time"
)

func TestWindowsFileLockSerializesAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.lock")
	release, err := flock(path)
	if err != nil {
		t.Fatal(err)
	}

	acquired := make(chan error, 1)
	go func() {
		unlock, lockErr := flock(path)
		if lockErr == nil {
			unlock()
		}
		acquired <- lockErr
	}()

	select {
	case err := <-acquired:
		release()
		t.Fatalf("second lock completed before release: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	release()
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second lock did not complete after release")
	}
}
