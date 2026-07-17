//go:build linux

package freeturn

import (
	"os/exec"
	"syscall"
)

// setProcessGroup detaches the child into its own session so a SIGKILL to
// the whole group can clean up any helpers it spawns (mirrors the
// Setsid:true used by internal/singbox.Process).
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// terminate sends SIGTERM (graceful stop).
func terminate(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

// kill sends SIGKILL (forced stop after the grace period).
func kill(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}

// isAlive probes process existence with signal 0 (sends nothing, just
// checks the kernel still knows the PID).
func isAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}
