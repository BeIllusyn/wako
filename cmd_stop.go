package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

func cmdStop(args []string) error {
	all := false
	var name string

	for _, arg := range args {
		switch {
		case arg == "-a" || arg == "--all":
			all = true
		case len(arg) > 0 && arg[0] == '-':
			return fmt.Errorf("unknown flag %q (usage: wako stop [--all] <run name>)", arg)
		default:
			if name != "" {
				return fmt.Errorf("unexpected argument %q (usage: wako stop [--all] <run name>)", arg)
			}
			name = arg
		}
	}

	if all {
		if name != "" {
			return errors.New("usage: wako stop --all (cannot combine with a run name)")
		}
		return stopAllRuns()
	}
	if name == "" {
		return errors.New("usage: wako stop <run name>")
	}

	if _, err := stopRun(name); err != nil {
		return err
	}
	fmt.Printf("stopped run %q\n", name)
	return nil
}

func stopAllRuns() error {
	runs, err := runningRuns()
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		fmt.Println("no running services")
		return nil
	}

	var firstErr error
	stopped := 0
	for _, st := range runs {
		forced, err := stopRun(st.Name)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			fmt.Fprintf(os.Stderr, "wako: %v\n", err)
			continue
		}
		stopped++
		if forced {
			fmt.Printf("stopped run %q (forced)\n", st.Name)
		} else {
			fmt.Printf("stopped run %q\n", st.Name)
		}
	}

	fmt.Printf("stopped %d of %d runs\n", stopped, len(runs))
	return firstErr
}

// stopRun stops the run with the given name and reports whether the service
// had to be killed after ignoring SIGTERM.
func stopRun(name string) (forced bool, err error) {
	st, err := loadRunState(name)
	if err != nil {
		return false, err
	}
	if st == nil {
		return false, runNotFoundError{name: name}
	}

	if st.Status != "running" || st.Addr == "" {
		ready, err := waitForRun(name, 5*time.Second)
		if err != nil {
			if ready != nil && ready.Status == "error" {
				return false, fmt.Errorf("run %q failed to start: %s", name, ready.Error)
			}
			return false, err
		}
		st = ready
	}

	conn, err := dialRunRetry(st.Addr, 2*time.Second)
	if err != nil {
		// The supervisor is gone; clean up whatever is left.
		if !processAlive(st.SupervisorPID) {
			if st.ChildPID > 0 {
				terminateGroup(st.ChildPID)
				time.Sleep(300 * time.Millisecond)
				killGroup(st.ChildPID)
			}
			cleanupRunOwned(st)
			return true, nil
		}
		if runDead(st) {
			cleanupRunOwned(st)
			return false, fmt.Errorf("run %q is not running", name)
		}
		return false, fmt.Errorf("connect to run %q: %w", name, err)
	}
	defer conn.Close()

	if err := writeFrame(conn, frameHello, []byte(st.Token)); err != nil {
		return false, fmt.Errorf("talk to run %q: %w", name, err)
	}
	if err := writeFrame(conn, frameStop, nil); err != nil {
		return false, fmt.Errorf("talk to run %q: %w", name, err)
	}
	// Signal the service directly too: this keeps stop fast even if the
	// supervisor is momentarily busy.
	if st.ChildPID > 0 {
		terminateGroup(st.ChildPID)
	}

	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	for {
		typ, _, err := readFrame(conn)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				forced = true
			}
			break
		}
		if typ == frameExit {
			break
		}
	}

	if forced {
		// The supervisor did not answer; stop things directly.
		if st.ChildPID > 0 {
			terminateGroup(st.ChildPID)
			time.Sleep(300 * time.Millisecond)
			killGroup(st.ChildPID)
		}
		killPID(st.SupervisorPID)
		cleanupRunOwned(st)
		return true, nil
	}

	waitGone(name, 3*time.Second)
	return false, nil
}
