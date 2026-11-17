//go:build darwin || linux

package git

import (
	"os/exec"
	"syscall"
)

// detach starts cmd in a new session, without a controlling terminal, and
// makes cancelling it stop its whole process group.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
