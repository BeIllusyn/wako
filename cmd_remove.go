package main

import (
	"errors"
	"fmt"
)

func cmdRemove(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: wako remove <name>")
	}
	name := args[0]

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if _, ok := cfg.Services[name]; !ok {
		return fmt.Errorf("service %q not found (run 'wako list' to see your services)", name)
	}

	delete(cfg.Services, name)
	if err := saveConfig(cfg); err != nil {
		return err
	}

	fmt.Printf("removed service %q\n", name)
	return nil
}
