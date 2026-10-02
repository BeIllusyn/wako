package main

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"time"
)

func cmdPs(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: wako ps")
	}

	runs, err := runningRuns()
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		fmt.Println("no running services")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RUN\tSERVICE\tSTATUS\tPID\tMODE\tSTARTED")
	for _, st := range runs {
		status := st.Status
		if st.Status == "error" {
			status = "error: " + st.Error
		}
		mode := "foreground"
		if st.Detached {
			mode = "background"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n",
			st.Name, st.Service, status, st.ChildPID, mode, st.StartedAt.Format(time.RFC3339))
	}
	return w.Flush()
}
