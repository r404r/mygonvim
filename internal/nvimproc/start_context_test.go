package nvimproc

import (
	"context"
	"testing"
	"time"
)

func TestStartContextCancellationStopsSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sess, err := StartContext(ctx, Options{Bin: testBin(t), Args: []string{"--clean", "-n"}})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	cancel()
	select {
	case <-sess.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled session remained active")
	}
}
