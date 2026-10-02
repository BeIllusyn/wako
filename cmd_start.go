package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func cmdStart(args []string) error {
	var (
		detach  bool
		runName string
		name    string
	)

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-d" || arg == "--detach":
			detach = true
		case arg == "-n" || arg == "--name":
			if i+1 >= len(args) {
				return errors.New("-n requires a run name")
			}
			i++
			runName = args[i]
		case strings.HasPrefix(arg, "-n="):
			runName = strings.TrimPrefix(arg, "-n=")
		case strings.HasPrefix(arg, "--name="):
			runName = strings.TrimPrefix(arg, "--name=")
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q (usage: wako start [-d] [-n <run name>] <name>)", arg)
		default:
			if name != "" {
				return fmt.Errorf("unexpected argument %q (usage: wako start [-d] [-n <run name>] <name>)", arg)
			}
			name = arg
		}
	}

	if name == "" {
		return errors.New("usage: wako start [-d] [-n <run name>] <name>")
	}
	if runName != "" && !validName(runName) {
		return fmt.Errorf("invalid run name %q (use letters, digits, '.', '_' or '-')", runName)
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	svc, ok := cfg.Services[name]
	if !ok {
		return fmt.Errorf("service %q not found (run 'wako list' to see your services)", name)
	}
	if err := checkServiceDir(name, svc); err != nil {
		return err
	}

	ready, err := startRun(name, svc, runName, detach)
	if err != nil {
		return err
	}

	if detach {
		fmt.Printf("started %q in the background as run %q (pid %d)\n", name, ready.Name, ready.ChildPID)
		fmt.Printf("stop it with: wako stop %s\n", ready.Name)
		return nil
	}

	fmt.Fprintf(os.Stderr, "wako: %q running as %q: %s (in %s)\n", name, ready.Name, svc.Command, svc.Dir)
	return attachToRun(ready, modeForeground)
}

// checkServiceDir makes sure a service's working directory still exists.
func checkServiceDir(name string, svc Service) error {
	info, err := os.Stat(svc.Dir)
	if err != nil {
		return fmt.Errorf("service %q directory: %w", name, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("service %q directory %s is not a directory", name, svc.Dir)
	}
	return nil
}

// startRun claims a run name, spawns the supervisor and waits until the
// service is up.
func startRun(service string, svc Service, requested string, detached bool) (*RunState, error) {
	st, err := claimRun(service, requested, svc, detached)
	if err != nil {
		return nil, err
	}

	if err := spawnSupervisor(st.Name); err != nil {
		cleanupRunOwned(st)
		return nil, fmt.Errorf("start run %q: %w", st.Name, err)
	}

	ready, err := waitForRun(st.Name, 10*time.Second)
	if err != nil {
		if current, _ := loadRunState(st.Name); current != nil && runDead(current) {
			cleanupRunOwned(current)
		}
		return nil, err
	}
	return ready, nil
}

func newRunState(runName, service string, svc Service, detached bool) (*RunState, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	return &RunState{
		Name:      runName,
		Service:   service,
		Command:   svc.Command,
		Dir:       svc.Dir,
		Status:    "starting",
		Detached:  detached,
		Token:     token,
		StartedAt: time.Now(),
	}, nil
}

// claimRun picks a free run name and claims it atomically. When requested is
// empty the service name is used, with -1, -2, ... appended on conflict.
func claimRun(service, requested string, svc Service, detached bool) (*RunState, error) {
	if requested != "" {
		existing, err := loadRunState(requested)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			removeEmptyState(requested)
		} else if runIsStale(existing) {
			cleanupRun(existing)
		} else {
			return nil, fmt.Errorf("run %q already exists (see 'wako ps')", requested)
		}

		st, err := newRunState(requested, service, svc, detached)
		if err != nil {
			return nil, err
		}
		if err := createRunState(st); err != nil {
			if errors.Is(err, errRunExists) {
				return nil, fmt.Errorf("run %q already exists (see 'wako ps')", requested)
			}
			return nil, err
		}
		return st, nil
	}

	for i := 0; i < 1000; i++ {
		candidate := service
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", service, i)
		}
		if runExists(candidate) {
			continue
		}
		st, err := newRunState(candidate, service, svc, detached)
		if err != nil {
			return nil, err
		}
		if err := createRunState(st); err == nil {
			return st, nil
		} else if !errors.Is(err, errRunExists) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("could not find a free run name for %q", service)
}

func spawnSupervisor(name string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, "__supervise", "-n", name)
	detachProcess(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// waitForRun waits until a run is up and running, or failed to start.
func waitForRun(name string, timeout time.Duration) (*RunState, error) {
	deadline := time.Now().Add(timeout)
	for {
		st, err := loadRunState(name)
		if err != nil {
			return nil, err
		}
		if st != nil {
			switch st.Status {
			case "running":
				if st.Addr != "" {
					return st, nil
				}
			case "error":
				return st, fmt.Errorf("run %q failed to start: %s", name, st.Error)
			}
		}

		if time.Now().After(deadline) {
			if st == nil {
				return nil, runNotFoundError{name: name}
			}
			return st, fmt.Errorf("timed out waiting for run %q to start", name)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func shellCommand() (string, string) {
	if isWindows() {
		return "cmd.exe", "/C"
	}
	return "/bin/sh", "-c"
}
