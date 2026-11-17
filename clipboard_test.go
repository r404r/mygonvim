package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestClipboardMainHonorsExpiredRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := onClipboardMain(ctx, func(fn func()) { cancel(); fn() }, func() error { calls++; return nil })
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("expired dispatch: calls=%d err=%v", calls, err)
	}
	err = onClipboardMain(context.Background(), func(fn func()) { fn() }, func() error { calls++; return errors.New("native failure") })
	if err == nil || err.Error() != "native failure" || calls != 1 {
		t.Fatalf("dispatch: calls=%d err=%v", calls, err)
	}
	if err := onClipboardMain(context.Background(), func(func()) {}, func() error { calls++; return nil }); err == nil || calls != 1 {
		t.Fatal("stopped loop reported success")
	}
}

func TestClipboardReaderProcess(t *testing.T) {
	mode := os.Getenv("MYGONVIM_TEST_CLIPBOARD_READER")
	if mode == "" {
		return
	}
	switch mode {
	case "text":
		fmt.Fprint(os.Stdout, "中文\r\n<CR>🙂")
		os.Exit(0)
	case "empty":
		os.Exit(0)
	case "failure":
		fmt.Fprint(os.Stderr, "native read denied")
		os.Exit(1)
	case "stall":
		time.Sleep(time.Hour)
	}
	os.Exit(2)
}

func TestNativeClipboardReaderDeadlineAndErrors(t *testing.T) {
	for _, mode := range []string{"text", "empty", "failure", "stall"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("MYGONVIM_TEST_CLIPBOARD_READER", mode)
			timeout := 5 * time.Second
			if mode == "stall" {
				timeout = 150 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			started := time.Now()
			text, err := readClipboardCommand(ctx, os.Args[0], "-test.run=^TestClipboardReaderProcess$")
			switch mode {
			case "text":
				if err != nil || text != "中文\r\n<CR>🙂" {
					t.Fatalf("text=%q err=%v", text, err)
				}
			case "empty":
				if err != nil || text != "" {
					t.Fatalf("empty text=%q err=%v", text, err)
				}
			case "failure":
				if text != "" || err == nil || !strings.Contains(err.Error(), "native read denied") {
					t.Fatalf("text=%q err=%v", text, err)
				}
			case "stall":
				if text != "" || !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > 2*time.Second {
					t.Fatalf("text=%q err=%v elapsed=%s", text, err, time.Since(started))
				}
			}
		})
	}
}
