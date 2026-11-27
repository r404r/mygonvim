//go:build !unix

package loginenv

import "context"

func runProcess(context.Context, Request) Output {
	return Output{ExitCode: -1, Failure: "platform unavailable"}
}
func StdoutTTY() bool { return false }
