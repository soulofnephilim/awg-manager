//go:build !linux

package freeturn

import (
	"os"
	"os/exec"
)

// These no-op / best-effort implementations exist only so the package
// compiles on a non-Linux dev machine (e.g. building awg-manager on
// Windows to check it compiles). FreeTurn itself only ever runs on the
// Linux/ARM router, where process_linux.go is used instead.

func setProcessGroup(_ *exec.Cmd) {
	// No process-group/session handling on non-Linux dev hosts.
}

func terminate(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func kill(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func isAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// FindProcess always succeeds on Windows; Signal(0) is the portable
	// liveness probe but isn't reliable cross-platform, so we treat a
	// successful FindProcess as "assume alive" — good enough for a dev
	// build that never actually runs the daemon.
	_, err := os.FindProcess(pid)
	return err == nil
}
