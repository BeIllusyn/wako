package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"
)

type attachMode int

const (
	modeForeground attachMode = iota
	modeResume
)

// attachToRun streams a running service's output to the terminal and its
// stdin to the service. In foreground mode Ctrl+C stops the service; in resume
// mode Ctrl+C only detaches, leaving the service running.
func attachToRun(st *RunState, mode attachMode) error {
	conn, err := dialRunRetry(st.Addr, 3*time.Second)
	if err != nil {
		if !runExists(st.Name) {
			return fmt.Errorf("run %q has already ended", st.Name)
		}
		return fmt.Errorf("connect to run %q: %w", st.Name, err)
	}
	defer conn.Close()

	if err := writeFrame(conn, frameHello, []byte(st.Token)); err != nil {
		return fmt.Errorf("connect to run %q: %w", st.Name, err)
	}

	var (
		stopRequested atomic.Bool
		detached      atomic.Bool
		stopOnce      sync.Once
		writeMu       sync.Mutex
	)

	sendFrame := func(typ byte, payload []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return writeFrame(conn, typ, payload)
	}

	// Forward terminal input to the service.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				if sendFrame(frameInput, buf[:n]) != nil {
					return
				}
			}
			if err != nil {
				_ = sendFrame(frameStdinClose, nil)
				return
			}
		}
	}()

	sigCh := make(chan os.Signal, 4)
	signal.Notify(sigCh, interruptSignals()...)
	defer signal.Stop(sigCh)
	go func() {
		for range sigCh {
			if mode == modeForeground {
				stopOnce.Do(func() {
					stopRequested.Store(true)
					// Signal the service directly as well: the supervisor may
					// be busy, and Ctrl+C should feel immediate.
					if st.ChildPID > 0 {
						interruptGroup(st.ChildPID)
					}
					_ = sendFrame(frameStop, nil)
				})
				continue
			}
			detached.Store(true)
			_ = conn.Close()
			return
		}
	}()

	for {
		typ, payload, err := readFrame(conn)
		if err != nil {
			if detached.Load() {
				fmt.Fprintf(os.Stderr, "wako: detached from run %q (still running)\n", st.Name)
				return nil
			}
			if stopRequested.Load() {
				return nil
			}
			if waitGone(st.Name, time.Second) {
				if mode == modeResume {
					fmt.Fprintf(os.Stderr, "wako: run %q ended\n", st.Name)
				}
				return nil
			}
			return fmt.Errorf("lost connection to run %q", st.Name)
		}

		switch typ {
		case frameOutput:
			if _, err := os.Stdout.Write(payload); err != nil {
				return err
			}
		case frameError:
			return errors.New(string(payload))
		case frameExit:
			var info exitInfo
			_ = json.Unmarshal(payload, &info)

			if stopRequested.Load() {
				return nil
			}
			if mode == modeResume {
				fmt.Fprintf(os.Stderr, "wako: run %q exited (code %d)\n", st.Name, info.Code)
				return nil
			}
			if info.Code == 0 {
				fmt.Fprintf(os.Stderr, "wako: run %q exited\n", st.Name)
				return nil
			}
			return exitCodeError{code: info.Code}
		}
	}
}

func dialRunRetry(addr string, timeout time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(timeout)
	for {
		conn, err := dialRun(addr)
		if err == nil {
			return conn, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(50 * time.Millisecond)
	}
}
