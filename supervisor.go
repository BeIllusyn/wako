package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"time"
)

const (
	// replayLimit caps the in-memory startup output that is replayed to the
	// first client, so attaching never misses an early banner. Nothing is
	// written to disk.
	replayLimit = 256 << 10

	// lingerTimeout lets a client that is about to attach still receive the
	// output and exit code of a short-lived service.
	lingerTimeout = 3 * time.Second

	// stopGrace is how long a service gets to exit after SIGTERM before it
	// is killed.
	stopGrace = 5 * time.Second

	// outputDrain is how long to wait for the output pipe to close after the
	// service exits.
	outputDrain = 2 * time.Second
)

// cmdSupervise runs the hidden per-run supervisor. It is spawned by
// 'wako start' and is what actually keeps the service alive.
func cmdSupervise(args []string) error {
	var name string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-n" || arg == "--name":
			if i+1 >= len(args) {
				return errors.New("usage: wako __supervise -n <run name>")
			}
			i++
			name = args[i]
		default:
			return fmt.Errorf("unexpected argument %q", arg)
		}
	}
	if name == "" {
		return errors.New("usage: wako __supervise -n <run name>")
	}
	return supervise(name)
}

type supervisor struct {
	st        *RunState
	hub       *hub
	stdin     io.WriteCloser
	stdinMu   sync.Mutex
	stopOnce  sync.Once
	killTimer *time.Timer
	timerMu   sync.Mutex
	readDone  chan struct{}
}

func supervise(name string) error {
	st, err := loadRunState(name)
	if err != nil {
		return err
	}
	if st == nil {
		return fmt.Errorf("run %q not found", name)
	}
	if st.Status != "starting" {
		return fmt.Errorf("run %q is already supervised", name)
	}

	// Any socket for a claimed name is left over from a crashed run.
	removeRunSocket(name)
	ln, addr, err := listenRun(name)
	if err != nil {
		return failRun(st, fmt.Errorf("listen: %w", err))
	}
	defer ln.Close()

	shell, shellFlag := shellCommand()
	cmd := exec.Command(shell, shellFlag, st.Command)
	cmd.Dir = st.Dir
	configureChild(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return failRun(st, err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		return failRun(st, err)
	}
	cmd.Stdout = outW
	cmd.Stderr = outW

	if err := cmd.Start(); err != nil {
		outW.Close()
		outR.Close()
		return failRun(st, fmt.Errorf("start command: %w", err))
	}
	outW.Close()

	s := &supervisor{
		st:       st,
		hub:      newHub(),
		stdin:    stdin,
		readDone: make(chan struct{}),
	}

	st.Addr = addr
	st.SupervisorPID = os.Getpid()
	st.ChildPID = cmd.Process.Pid
	st.Status = "running"
	if err := saveRunState(st); err != nil {
		_ = cmd.Process.Kill()
		return err
	}

	go s.readOutput(outR)
	go s.acceptLoop(ln)

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, interruptSignals()...)
	defer signal.Stop(sigCh)
	go func() {
		for range sigCh {
			s.requestStop()
		}
	}()

	waitErr := cmd.Wait()
	code := exitCodeOf(waitErr)

	s.timerMu.Lock()
	if s.killTimer != nil {
		s.killTimer.Stop()
	}
	s.timerMu.Unlock()

	// Give a client that is about to attach a chance to catch the tail of the
	// output and the exit code before the run disappears.
	select {
	case <-s.hub.first:
	case <-time.After(lingerTimeout):
	}

	select {
	case <-s.readDone:
	case <-time.After(outputDrain):
	}

	s.hub.broadcast(frameExit, exitPayload(code))
	s.hub.closeAll()

	cleanupRunOwned(st)
	removeRunSocket(name)
	return nil
}

// failRun records why a run could not start, so 'wako start' can report it.
func failRun(st *RunState, cause error) error {
	st.Status = "error"
	st.Error = cause.Error()
	_ = saveRunState(st)
	return cause
}

func (s *supervisor) requestStop() {
	s.stopOnce.Do(func() {
		pid := s.st.ChildPID
		if pid <= 0 {
			return
		}
		terminateGroup(pid)

		s.timerMu.Lock()
		s.killTimer = time.AfterFunc(stopGrace, func() { killGroup(pid) })
		s.timerMu.Unlock()
	})
}

func (s *supervisor) readOutput(r io.ReadCloser) {
	defer close(s.readDone)
	defer r.Close()

	buf := make([]byte, 32<<10)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			s.hub.publishOutput(chunk)
		}
		if err != nil {
			return
		}
	}
}

func (s *supervisor) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handleClient(conn)
	}
}

func (s *supervisor) handleClient(conn net.Conn) {
	// The first frame must prove the client can read the run's state file.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	typ, payload, err := readFrame(conn)
	if err != nil || typ != frameHello || string(payload) != s.st.Token {
		_ = conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	c := s.hub.add(conn)
	defer func() {
		s.hub.remove(c)
		_ = conn.Close()
	}()

	for {
		typ, payload, err := readFrame(conn)
		if err != nil {
			return
		}
		switch typ {
		case frameInput:
			s.stdinMu.Lock()
			_, werr := s.stdin.Write(payload)
			s.stdinMu.Unlock()
			if werr != nil {
				return
			}
		case frameStdinClose:
			s.stdinMu.Lock()
			_ = s.stdin.Close()
			s.stdinMu.Unlock()
		case frameStop:
			s.requestStop()
		}
	}
}

// hub fans service output out to attached clients. It also keeps the startup
// output in memory until the first client attaches, so that output produced
// before the client connected is not lost.
type hub struct {
	mu        sync.Mutex
	clients   map[*client]struct{}
	buffer    []byte
	capture   bool
	first     chan struct{}
	firstOnce sync.Once
}

type client struct {
	conn net.Conn
	mu   sync.Mutex
}

func newHub() *hub {
	return &hub{
		clients: map[*client]struct{}{},
		capture: true,
		first:   make(chan struct{}),
	}
}

func (h *hub) add(conn net.Conn) *client {
	c := &client{conn: conn}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.clients[c] = struct{}{}

	replay := h.buffer
	h.buffer = nil
	h.capture = false

	h.firstOnce.Do(func() { close(h.first) })

	if len(replay) > 0 {
		_ = c.write(frameOutput, replay)
	}
	return c
}

func (h *hub) remove(c *client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

// publishOutput records a chunk for replay and forwards it to current clients.
// Both happen under one lock so a client attaching concurrently gets the chunk
// exactly once (either in its replay or as a live frame, never both).
func (h *hub) publishOutput(chunk []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.capture {
		h.buffer = append(h.buffer, chunk...)
		if len(h.buffer) > 2*replayLimit {
			h.buffer = append([]byte(nil), h.buffer[len(h.buffer)-replayLimit:]...)
		}
	}

	for c := range h.clients {
		if err := c.write(frameOutput, chunk); err != nil {
			delete(h.clients, c)
			_ = c.conn.Close()
		}
	}
}

func (h *hub) broadcast(typ byte, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for c := range h.clients {
		if err := c.write(typ, payload); err != nil {
			delete(h.clients, c)
			_ = c.conn.Close()
		}
	}
}

func (h *hub) closeAll() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for c := range h.clients {
		_ = c.conn.Close()
		delete(h.clients, c)
	}
}

func (c *client) write(typ byte, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	_ = c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if err := writeFrame(c.conn, typ, payload); err != nil {
		return err
	}
	_ = c.conn.SetWriteDeadline(time.Time{})
	return nil
}
