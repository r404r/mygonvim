package main

import (
	"context"
	"errors"
	"log"

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

func readClipboardForPaste() string {
	text, err := readClipboardText()
	if err != nil {
		log.Printf("clipboard paste: %v", err)
		return ""
	}
	return text
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
		Read: func(ctx context.Context) (text string, err error) {
			err = onClipboardMain(ctx, mygo.RunOnMain, func() error {
				var e error
				text, e = readClipboardText()
				return e
			})
			return
		},
		Write: func(ctx context.Context, text string) error {
			return onClipboardMain(ctx, mygo.RunOnMain, func() error { return mygo.Clipboard.Write(transfer.TextData(text)) })
		},
	}
}
