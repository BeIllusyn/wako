//go:build windows

package main

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"syscall"
)

func isWindows() bool { return true }

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
)

func configureChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
}

func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | detachedProcess}
}

func interruptSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

// Windows has no Unix sockets; the control endpoint is a loopback TCP port.
func listenRun(name string) (net.Listener, string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	return ln, ln.Addr().String(), nil
}

func dialRun(addr string) (net.Conn, error) {
	return net.Dial("tcp", addr)
}

func terminatePID(pid int) { killPID(pid) }
func killPID(pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}

func terminateGroup(pid int) { killPID(pid) }
func killGroup(pid int)      { killPID(pid) }
func interruptGroup(pid int) { killPID(pid) }

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	syscall.CloseHandle(h)
	return true
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}
