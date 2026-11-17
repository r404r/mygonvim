//go:build !windows

package main

import "os/exec"

func hideClipboardReader(*exec.Cmd) {}
