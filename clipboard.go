package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/r404r/mygonvim/internal/nvimproc"
)

func readClipboardText() (string, error) {
	b, err := mygo.Clipboard.ReadFormat(transfer.Text)
	if errors.Is(err, transfer.ErrFormat) {
		return "", nil
	}
	return string(b), err
}

const clipboardReadArgument = "--clipboard-read"

// Native clipboard calls can wait indefinitely for another application's data.
// Isolate reads in a windowless process so the deadline can stop the native
// event loop itself, rather than leaving the editor's main thread blocked.
func readNativeClipboard(ctx context.Context) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return readClipboardCommand(ctx, exe, clipboardReadArgument)
}

func readClipboardCommand(ctx context.Context, exe string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	hideClipboardReader(cmd)
	data, err := cmd.Output()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		if failure, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("native clipboard reader: %w: %s", err, strings.TrimSpace(string(failure.Stderr)))
		}
		return "", err
	}
	return string(data), nil
}

// This private entry point initializes only MyGo's clipboard, without an
// editor, Neovim process, console window or Dock icon.
func runClipboardReader() int {
	var readErr error
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyProhibited)
	mygo.App.WhenReady(func() {
		var text string
		text, readErr = readClipboardText()
		if readErr == nil {
			_, readErr = fmt.Fprint(os.Stdout, text)
		}
		mygo.App.Quit()
	})
	if err := mygo.App.Run(); err != nil {
		readErr = err
	}
	if readErr != nil {
		fmt.Fprintln(os.Stderr, readErr)
		return 1
	}
	return 0
}

// Test the deadline again when the UI actually dispatches the operation: a
// timed-out RPC must not overwrite a newer clipboard after the UI unblocks.
func onClipboardMain(ctx context.Context, run func(func()), fn func() error) error {
	err := errors.New("clipboard event loop stopped")
	run(func() {
		if err = ctx.Err(); err == nil {
			err = fn()
		}
	})
	return err
}

func systemClipboard() *nvimproc.Clipboard {
	return &nvimproc.Clipboard{
		Read: readNativeClipboard,
		Write: func(ctx context.Context, text string) error {
			return onClipboardMain(ctx, mygo.RunOnMain, func() error { return mygo.Clipboard.Write(transfer.TextData(text)) })
		},
	}
}
