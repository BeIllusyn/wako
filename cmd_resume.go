package main

import (
	"errors"
	"fmt"
	"time"
)

func cmdResume(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: wako resume <run name>")
	}
	name := args[0]

	st, err := waitForRun(name, 3*time.Second)
	if err != nil {
		if st != nil && st.Status == "error" {
			return fmt.Errorf("run %q failed to start: %s", name, st.Error)
		}
		return err
	}

	return attachToRun(st, modeResume)
}
