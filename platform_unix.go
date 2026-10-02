//go:build !windows

package main

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"syscall"
)

func isWindows() bool { return false }

// configureChild puts the service in its own process group so it can be
// signalled (together with anything it spawns) without touching wako.
func configureChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// detachProcess starts a process in a new session, detached from the terminal.
func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// interruptSignals are the signals that mean "the user wants out".
func interruptSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
}

// listenRun opens the control endpoint for a run and returns its address.
func listenRun(name string) (net.Listener, string, error) {
	path, err := socketPath(name)
	if err != nil {
		return nil, "", err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, "", err
	}
	_ = os.Chmod(path, 0o600)
	return ln, path, nil
}

func dialRun(addr string) (net.Conn, error) {
	return net.Dial("unix", addr)
}

func terminatePID(pid int) { _ = syscall.Kill(pid, syscall.SIGTERM) }
func killPID(pid int)      { _ = syscall.Kill(pid, syscall.SIGKILL) }

func terminateGroup(pgid int) { _ = syscall.Kill(-pgid, syscall.SIGTERM) }
func killGroup(pgid int)      { _ = syscall.Kill(-pgid, syscall.SIGKILL) }
func interruptGroup(pgid int) { _ = syscall.Kill(-pgid, syscall.SIGINT) }

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 1
	}
	if code := exitErr.ExitCode(); code >= 0 {
		return code
	}
	if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return 1
}
