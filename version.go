package main

import (
	"errors"
	"fmt"
)

// These are set at build time by GoReleaser (see .goreleaser.yaml). A plain
// 'go build' or 'go install' keeps the defaults below.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func cmdVersion(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: wako version")
	}
	fmt.Printf("wako %s (commit %s, built %s)\n", version, commit, date)
	return nil
}
