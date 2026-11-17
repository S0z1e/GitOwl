//go:build !darwin && !linux

package git

import "os/exec"

func detach(*exec.Cmd) {}
