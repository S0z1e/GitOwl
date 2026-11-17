package git

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"
)

// RunNetwork runs a Git command that talks to a remote, such as fetch or
// push. Git runs in a session of its own, without a terminal, so neither
// Git nor SSH can wait on a prompt nobody sees: a missing credential, an
// encrypted key without an agent or an unknown host key fails at once.
// When ctx ends, Git and everything it started (ssh, credential helpers)
// are stopped. It returns standard output even when Git fails, as push
// reports refused refs there.
func (r *Runner) RunNetwork(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if r == nil || r.Path == "" {
		return nil, ErrNotFound
	}
	env := r.env()
	env = append(env, "SSH_ASKPASS_REQUIRE=never", "GCM_INTERACTIVE=never")
	if r.batchSSH(ctx, dir, env) {
		env = append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	cmd := exec.CommandContext(ctx, r.Path, append(slices.Clone(global), args...)...)
	cmd.Dir = dir
	cmd.Env = env
	detach(cmd)
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), nil
	}
	if ctx.Err() != nil {
		return stdout.Bytes(), ctx.Err()
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return stdout.Bytes(), &Error{Args: args, ExitCode: exit.ExitCode(), Stderr: Redact(stderr.String())}
	}
	if errors.Is(err, exec.ErrNotFound) {
		return nil, ErrNotFound
	}
	if _, serr := os.Stat(r.Path); serr != nil {
		return nil, ErrNotFound
	}
	return nil, err
}

// batchSSH reports whether GitOwl may choose the SSH command: only when
// neither the environment nor the repository's configuration names one,
// so a user's own SSH setup always wins.
func (r *Runner) batchSSH(ctx context.Context, dir string, env []string) bool {
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if name == "GIT_SSH_COMMAND" || name == "GIT_SSH" {
			return false
		}
	}
	out, _, err := r.RunStatus(ctx, dir, nil, []int{1}, "config", "--get", "core.sshCommand")
	return err == nil && strings.TrimSpace(string(out)) == ""
}

// userinfo matches the user and password of a URL, as in
// https://user:token@host/.
var userinfo = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)([^/@\s]+)@`)

// Redact removes credentials from URLs in text, such as Git's error output
// for a remote whose URL contains a token. A bare user name of an SSH URL
// is kept; anything else before the host is replaced.
func Redact(text string) string {
	return userinfo.ReplaceAllStringFunc(text, func(m string) string {
		sub := userinfo.FindStringSubmatch(m)
		scheme, info := sub[1], sub[2]
		if strings.HasPrefix(strings.ToLower(scheme), "ssh") && !strings.Contains(info, ":") {
			return m
		}
		return scheme + "***@"
	})
}
