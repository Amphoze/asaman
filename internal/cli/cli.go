// Package cli dispatches asaman subcommands.
package cli

import "fmt"

const usage = `asaman — Agentic Session Assist MANagement
usage: asaman <command> [args]
commands: search sessions mem feedback index doctor setup serve import-csess version`

// Run dispatches args and returns a process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Println(usage)
		return 0
	}
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Println("asaman " + Version)
		return 0
	default:
		fmt.Println(usage)
		return 2
	}
}

// Version is the build version.
var Version = "0.1.0-dev"
