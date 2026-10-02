package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

func cmdAdd(args []string) error {
	replace := false
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-r", "--replace":
			replace = true
		default:
			return fmt.Errorf("unknown flag %q (usage: wako add [-r] <name> <command>)", args[0])
		}
		args = args[1:]
	}

	if len(args) < 2 {
		return errors.New("usage: wako add [-r] <name> <command>")
	}

	name := args[0]
	command := strings.Join(args[1:], " ")

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current directory: %w", err)
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	_, exists := cfg.Services[name]
	if exists && !replace {
		return fmt.Errorf("service %q already exists (use 'wako add -r' to replace it)", name)
	}

	cfg.Services[name] = Service{Command: command, Dir: dir}
	if err := saveConfig(cfg); err != nil {
		return err
	}

	verb := "added"
	if exists {
		verb = "replaced"
	}
	fmt.Printf("%s service %q: %s (in %s)\n", verb, name, command, dir)
	return nil
}
