package main

import (
	"context"
	"errors"
	"testing"
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
