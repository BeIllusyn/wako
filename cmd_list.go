package main

import (
	"fmt"
	"os"
	"text/tabwriter"
)

func cmdList(args []string) error {
	all := false
	for _, arg := range args {
		switch arg {
		case "-a", "--all":
			all = true
		default:
			return fmt.Errorf("unknown flag %q (usage: wako list [-a])", arg)
		}
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	names := cfg.Names()
	if len(names) == 0 {
		fmt.Println("no services registered yet (add one with 'wako add <name> <command>')")
		return nil
	}

	if !all {
		for _, name := range names {
			fmt.Println(name)
		}
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tCOMMAND\tDIR")
	for _, name := range names {
		svc := cfg.Services[name]
		fmt.Fprintf(w, "%s\t%s\t%s\n", name, svc.Command, svc.Dir)
	}
	return w.Flush()
}
