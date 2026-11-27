//go:build unix

package nvimproc

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStartContextCancelsPendingHandshake(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "nvim")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexec sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if sess, err := StartContext(ctx, Options{Bin: bin}); err == nil {
		sess.Close()
		t.Fatal("unresponsive executable completed handshake")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("pending startup ignored cancellation")
	}
}
