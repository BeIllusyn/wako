package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// cmdRestart stops a running service and starts it again. The name may be a
// registered service or a running run name; the service's current command and
// directory are used, so changes made with 'wako add -r' take effect.
func cmdRestart(args []string) error {
	var (
		detachFlag bool
		runName    string
		name       string
	)

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-d" || arg == "--detach":
			detachFlag = true
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
			return fmt.Errorf("unknown flag %q (usage: wako restart [-d] [-n <run name>] <service or run name>)", arg)
		default:
			if name != "" {
				return fmt.Errorf("unexpected argument %q (usage: wako restart [-d] [-n <run name>] <service or run name>)", arg)
			}
			name = arg
		}
	}

	if name == "" {
		return errors.New("usage: wako restart [-d] [-n <run name>] <service or run name>")
	}
	if runName != "" && !validName(runName) {
		return fmt.Errorf("invalid run name %q (use letters, digits, '.', '_' or '-')", runName)
	}

	// The run we are going to restart.
	target := name
	if runName != "" {
		target = runName
	}
	st, err := loadRunState(target)
	if err != nil {
		return err
	}
	if st != nil && runDead(st) {
		cleanupRun(st)
		st = nil
	}

	// Prefer the registered service so the latest command is used; fall back
	// to the run's recorded command for runs whose service is gone.
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	var (
		service string
		svc     Service
	)
	if registered, ok := cfg.Services[name]; ok {
		service = name
		svc = registered
	} else if st != nil {
		service = st.Service
		svc = Service{Command: st.Command, Dir: st.Dir}
	} else {
		return fmt.Errorf("service or run %q not found (run 'wako list' and 'wako ps')", name)
	}
	if err := checkServiceDir(service, svc); err != nil {
		return err
	}

	detach := detachFlag
	if st != nil {
		if !detachFlag {
			detach = st.Detached
		}
		if _, err := stopRun(target); err != nil {
			return fmt.Errorf("stop run %q before restart: %w", target, err)
		}
		if !waitGone(target, 3*time.Second) {
			return fmt.Errorf("run %q did not stop in time", target)
		}
	}

	// Reuse the stopped run's name so the restart keeps its identity.
	requested := runName
	if requested == "" && st != nil {
		requested = st.Name
	}

	ready, err := startRun(service, svc, requested, detach)
	if err != nil {
		return err
	}

	if detach {
		fmt.Printf("restarted %q in the background as run %q (pid %d)\n", name, ready.Name, ready.ChildPID)
		return nil
	}

	fmt.Fprintf(os.Stderr, "wako: %q restarted as %q: %s (in %s)\n", name, ready.Name, svc.Command, svc.Dir)
	return attachToRun(ready, modeForeground)
}
