package nvimproc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStartContextCancellationPreservesGracefulClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sess, err := StartContext(ctx, Options{Bin: testBin(t), Args: []string{"--clean", "-n"}})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	marker := filepath.Join(t.TempDir(), "left.txt")
	if err := sess.Nvim().ExecLua(`local path = ...; vim.api.nvim_create_autocmd('VimLeavePre', {callback = function() vim.fn.writefile({'left'}, path) end})`, nil, marker); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-sess.Done():
		t.Fatal("startup cancellation killed an attached session")
	case <-time.After(50 * time.Millisecond):
	}
	var alive int
	if err := sess.Nvim().ExecLua("return 42", &alive); err != nil || alive != 42 {
		t.Fatal("attached session did not survive cancellation")
	}
	sess.Close()
	if data, err := os.ReadFile(marker); err != nil || string(data) != "left\n" {
		t.Fatal("VimLeavePre did not run on Close after context cancellation")
	}
}

func TestStartContextAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := StartContext(ctx, Options{Bin: "missing-nvim"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled start returned %v", err)
	}
}
