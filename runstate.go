package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var errRunExists = errors.New("run already exists")

// RunState is the on-disk record of one running service instance.
type RunState struct {
	Name          string    `json:"name"`
	Service       string    `json:"service"`
	Command       string    `json:"command"`
	Dir           string    `json:"dir"`
	Status        string    `json:"status"` // starting | running | error
	Error         string    `json:"error,omitempty"`
	Detached      bool      `json:"detached,omitempty"`
	SupervisorPID int       `json:"supervisor_pid"`
	ChildPID      int       `json:"child_pid"`
	Addr          string    `json:"addr"`
	Token         string    `json:"token,omitempty"`
	StartedAt     time.Time `json:"started_at"`
}

// runNotFoundError reports an unknown run name.
type runNotFoundError struct {
	name string
}

func (e runNotFoundError) Error() string {
	return fmt.Sprintf("run %q not found (see running runs with 'wako ps')", e.name)
}

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// validName reports whether name is safe to use as a run name and file name.
func validName(name string) bool {
	return len(name) <= 64 && namePattern.MatchString(name)
}

func runsDir() (string, error) {
	path, err := configPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "runs"), nil
}

func runStatePath(name string) (string, error) {
	dir, err := runsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".json"), nil
}

// socketPath returns where a run's control socket lives. Unix socket paths are
// limited to ~104 bytes, so long paths fall back to a hashed file name.
func socketPath(name string) (string, error) {
	dir, err := runsDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name+".sock")
	if len(path) > 100 {
		sum := sha256.Sum256([]byte(name))
		path = filepath.Join(dir, "sock-"+hex.EncodeToString(sum[:8])+".sock")
	}
	return path, nil
}

func loadRunState(name string) (*RunState, error) {
	path, err := runStatePath(name)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		// The name is claimed but the state has not been written yet.
		return nil, nil
	}

	var st RunState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &st, nil
}

func saveRunState(st *RunState) error {
	path, err := runStatePath(st.Name)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, st.Name+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// createRunState claims a run name exclusively and writes the state in one
// step, so a failed claim never leaves an empty file behind. It uses a hard
// link because linking fails if the target already exists.
func createRunState(st *RunState) error {
	path, err := runStatePath(st.Name)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, st.Name+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}

	if err := os.Link(tmpName, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("run %q already exists: %w", st.Name, errRunExists)
		}
		return err
	}
	return nil
}

// removeEmptyState deletes a state file left with no content by an older or
// interrupted run. Such a file would otherwise block the name forever.
func removeEmptyState(name string) {
	path, err := runStatePath(name)
	if err != nil {
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() > 0 {
		return
	}
	_ = os.Remove(path)
}

func removeRunState(name string) {
	if path, err := runStatePath(name); err == nil {
		_ = os.Remove(path)
	}
}

func removeRunSocket(name string) {
	if path, err := socketPath(name); err == nil {
		_ = os.Remove(path)
	}
}

func cleanupRun(st *RunState) {
	removeRunState(st.Name)
	removeRunSocket(st.Name)
}

// cleanupRunOwned removes a run's state only if the name still refers to the
// same run, so a supervisor never deletes a newer run that reused the name.
func cleanupRunOwned(st *RunState) {
	current, err := loadRunState(st.Name)
	if err != nil || current == nil {
		return
	}
	if current.Token != "" && st.Token != "" && current.Token != st.Token {
		return
	}
	cleanupRun(current)
}

// runningRuns returns all live runs, oldest first. Leftover states from dead
// runs are cleaned up along the way.
func runningRuns() ([]*RunState, error) {
	dir, err := runsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var runs []*RunState
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		st, err := loadRunState(name)
		if err != nil || st == nil {
			continue
		}
		if runDead(st) {
			if runIsStale(st) {
				cleanupRun(st)
			}
			continue
		}
		runs = append(runs, st)
	}

	sort.Slice(runs, func(i, j int) bool { return runs[i].StartedAt.Before(runs[j].StartedAt) })
	return runs, nil
}

// waitGone reports whether a run's state is gone before the timeout. Dead
// leftovers are cleaned up along the way.
func waitGone(name string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		st, err := loadRunState(name)
		if err != nil || st == nil {
			return true
		}
		if runDead(st) {
			cleanupRun(st)
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// runDead reports whether both the supervisor and the service are gone.
func runDead(st *RunState) bool {
	if processAlive(st.SupervisorPID) {
		return false
	}
	return st.ChildPID == 0 || !processAlive(st.ChildPID)
}

// runIsStale reports whether a state file is left over from a dead run. A
// short grace period keeps concurrent starts from reaping each other's state
// while the supervisor is still coming up.
func runIsStale(st *RunState) bool {
	if !runDead(st) {
		return false
	}
	return time.Since(st.StartedAt) > 10*time.Second
}

func runExists(name string) bool {
	st, err := loadRunState(name)
	if err != nil || st == nil {
		return false
	}
	if runIsStale(st) {
		cleanupRun(st)
		return false
	}
	return true
}

// nextRunName returns base if it is free, otherwise base-1, base-2, ...
func nextRunName(base string) (string, error) {
	if !runExists(base) {
		return base, nil
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !runExists(candidate) {
			return candidate, nil
		}
	}
}

func randomToken() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
