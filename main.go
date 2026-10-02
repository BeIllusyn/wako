// Command wako is a small CLI that remembers how to start your services.
//
// Register a service from the directory it lives in:
//
//	wako add web pnpm dev
//
// Then start it from anywhere:
//
//	wako start web
package main

import (
	"errors"
	"fmt"
	"os"
)

// exitCodeError carries the exit status of a service so wako can mirror it.
type exitCodeError struct {
	code int
}

func (e exitCodeError) Error() string {
	return fmt.Sprintf("service exited with status %d", e.code)
}

func main() {
	err := run(os.Args[1:])
	if err == nil {
		return
	}

	fmt.Fprintln(os.Stderr, "wako: "+err.Error())

	var code exitCodeError
	if errors.As(err, &code) {
		os.Exit(code.code)
	}
	os.Exit(1)
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	command, rest := args[0], args[1:]
	switch command {
	case "add":
		return cmdAdd(rest)
	case "remove", "rm":
		return cmdRemove(rest)
	case "start":
		return cmdStart(rest)
	case "stop":
		return cmdStop(rest)
	case "restart":
		return cmdRestart(rest)
	case "resume":
		return cmdResume(rest)
	case "ps", "runs":
		return cmdPs(rest)
	case "list", "ls":
		return cmdList(rest)
	case "version", "--version":
		return cmdVersion(rest)
	case "__supervise":
		return cmdSupervise(rest)
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q (run 'wako help' for usage)", command)
	}
}

func printUsage() {
	fmt.Print(`wako — start your services with one command

Usage:
  wako add [-r] <name> <command>          Register a service (remembers the current directory)
  wako remove <name>                      Remove a service
  wako start [-d] [-n <run name>] <name>  Start a service
  wako stop [--all] [run name]            Stop a running service, or all of them
  wako restart [-d] [-n <run name>] <name>  Restart a service with its current command
  wako resume <run name>                  Attach to a running service (Ctrl+C detaches)
  wako ps                                 List running services
  wako list [-a]                          List services; -a also shows commands and directories
  wako version                            Show the wako version
  wako help                               Show this help

Starting services:
  Without -d, wako stays attached: you see the output and Ctrl+C stops the service.
  With -d, the service keeps running in the background after wako exits.
  Every run gets a name: the service name, or <service>-1, <service>-2, ... when it
  is taken. Use -n to choose your own.

Examples:
  wako add web pnpm dev
  wako add -r web pnpm dev
  wako start web                 # foreground run named "web"
  wako start -d web              # background run named "web" (or "web-1" if taken)
  wako start -d -n web2 web      # background run named "web2"
  wako resume web-1              # attach; Ctrl+C detaches without stopping it
  wako restart web               # stop "web" and start it again
  wako stop web-1                # stop the run named "web-1"
  wako stop --all                # stop every running service
  wako ps                        # see running services and their run names
`)
}
