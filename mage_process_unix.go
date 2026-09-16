// SPDX-License-Identifier: AGPL-3.0-or-later
//go:build mage && !windows

package main

import (
	"os/exec"
	"syscall"
)

// setProcessGroup configures a command to run in its own process group,
// so that all child processes can be killed together.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends a signal to the entire process group of the given command.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil { // use best-effort to kill full process group
		err = syscall.Kill(-pgid, syscall.SIGTERM)
		if err != nil {
			return err
		}
	}
	return cmd.Wait()
}
