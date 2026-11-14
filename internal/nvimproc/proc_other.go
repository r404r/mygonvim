//go:build !windows

package nvimproc

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
