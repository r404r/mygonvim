package loginenv

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const Timeout = 5 * time.Second
const outputLimit = 1 << 20
const waitDelay = 200 * time.Millisecond

// Request contains no command assembled from user input. Only Script's random
// hexadecimal marker is interpolated into the shell command.
type Request struct {
	Path      string
	Args, Env []string
	Dir       string
}

type Output struct {
	Stdout   []byte
	ExitCode int
	Failure  string
}

type Runner func(context.Context, Request) Output

type Options struct {
	Inputs
	Home, Username string
	Run            Runner
	Logf           func(string, ...any)
}

type Result struct {
	Env    []string
	Status string
}

// SettingsOff leaves a missing, damaged, or non-boolean setting at its default.
func SettingsOff(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return false
	}
	var enabled bool
	return string(fields["loginEnv"]) != "null" && json.Unmarshal(fields["loginEnv"], &enabled) == nil && !enabled
}

func Script(marker string) string {
	return "printf %s " + marker + " && /usr/bin/env -0 && printf %s " + marker
}

// Import never returns a shell error or its output to a caller or logger.
// Every failure returns the original environment, unchanged.
func Import(parent context.Context, o Options) Result {
	result := Result{Env: append([]string{}, o.Env...)}
	should, reason := Decide(o.Inputs)
	result.Status = "Skipped: " + reason
	if !should {
		return result
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(parent, Timeout)
	defer cancel()
	run := o.Run
	if run == nil {
		run = runProcess
	}
	shell := Value(o.Env, "SHELL")
	if shell == "" || !executable(shell) {
		shell = ""
		if o.Username != "" && !strings.ContainsAny(o.Username, "/\x00\r\n") {
			out := run(ctx, Request{Path: "/usr/bin/dscl", Args: []string{".", "-read", "/Users/" + o.Username, "UserShell"}, Env: o.Env, Dir: o.Home})
			if out.Failure == "" && out.ExitCode == 0 {
				if value, ok := strings.CutPrefix(strings.TrimSpace(string(out.Stdout)), "UserShell:"); ok {
					shell = strings.TrimSpace(value)
				}
			}
		}
		if shell == "" {
			shell = "/bin/zsh"
		}
	}
	failure, exitCode := "", -1
	vars := map[string]string{}
	st, err := os.Stat(shell)
	if err != nil || !ValidateShell(shell, st.Mode()) {
		failure = "unsupported shell"
	} else if o.Home == "" || !filepath.IsAbs(o.Home) {
		failure = "home unavailable"
	} else {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			failure = "marker unavailable"
		} else {
			marker := hex.EncodeToString(random[:])
			env := append(append([]string{}, o.Env...), "MYGONVIM_LOGIN_ENV_PROBE=1")
			out := run(ctx, Request{Path: shell, Args: []string{"-l", "-i", "-c", Script(marker)}, Env: env, Dir: o.Home})
			failure, exitCode = out.Failure, out.ExitCode
			if failure == "" {
				if parsed, err := Parse(out.Stdout, marker); err != nil {
					failure = "invalid snapshot"
				} else {
					vars = parsed
				}
			}
		}
	}
	if ctx.Err() != nil {
		failure = "cancelled"
		if parent.Err() == nil {
			failure = "timeout"
		}
	}
	result.Status = "Imported"
	if failure != "" {
		result.Status = "Fallback: " + failure
	} else {
		result.Env = Merge(o.Env, vars)
	}
	if o.Logf != nil {
		o.Logf("login env: attempts=1 status=%q elapsed_ms=%d shell=%q exit=%d variables=%d path_entries=%d", result.Status, time.Since(start).Milliseconds(), safeShellName(shell), exitCode, len(vars), pathCount(Value(result.Env, "PATH")))
		if failure == "" && Value(o.Env, "MYGONVIM_LOGIN_ENV_DEBUG") == "1" {
			keys := make([]string, 0, len(vars))
			for key := range vars {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			o.Logf("login env: names=%q", keys)
		}
	}
	return result
}

func executable(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0o111 != 0
}
func safeShellName(path string) string {
	name := filepath.Base(path)
	if name == "bash" || name == "zsh" {
		return name
	}
	return "unsupported"
}
func pathCount(path string) int {
	if path == "" {
		return 0
	}
	return len(strings.Split(path, ":"))
}
