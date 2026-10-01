// Command dot copies a git-backed setup to the destinations in dot.toml.
package main

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/fschrhunt/dot/internal/app"
	"github.com/fschrhunt/dot/internal/config"
	"github.com/fschrhunt/dot/internal/schedule"
	"github.com/fschrhunt/dot/internal/setup"
	dotsync "github.com/fschrhunt/dot/internal/sync"
)

var version = "dev"

// dispatch parses command arguments and calls the appropriate handler.
func dispatch(args []string, out, stderr io.Writer) (int, error) {
	command := "status"
	if len(args) > 0 {
		command, args = args[0], args[1:]
	}
	paths := setup.FromEnv()
	first := ""
	if len(args) > 0 {
		first = args[0]
	}
	switch command {
	case "version", "--version":
		fmt.Fprintln(out, version)
		return 0, nil
	case "help", "-h", "--help":
		return app.Help(paths, out)
	case "init":
		return app.Init(paths, first, out, stderr)
	case "sync":
		return dotsync.Run(paths)
	}
	c, e := config.Load(paths)
	if e != nil {
		return 2, e
	}
	switch command {
	case "status":
		return app.Status(c, first, out)
	case "install":
		return schedule.Run(paths, slices.Contains(args, "--remove"), out)
	case "take":
		if len(args) != 1 {
			return 2, setup.Fail("usage: dot take <live-path>")
		}
		return app.Take(c, first, out)
	case "apply":
		var unknown []string
		for _, s := range args {
			if s != "-n" && s != "--force" {
				unknown = append(unknown, s)
			}
		}
		if len(unknown) > 0 {
			slices.Sort(unknown)
			return 2, setup.Fail("unknown option " + unknown[0] + " (dot apply [-n] [--force])")
		}
		return app.Apply(c, slices.Contains(args, "-n"), slices.Contains(args, "--force"), out, stderr)
	}
	return 2, setup.Fail("unknown command " + command + " (see dot help)")
}

// main prints each user problem on one prefixed line and exits with the handler's code.
func main() {
	code, err := dispatch(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		for _, line := range strings.Split(strings.TrimRight(err.Error(), "\n"), "\n") {
			fmt.Fprintln(os.Stderr, "dot: "+line)
		}
		code = setup.ExitCode(err)
	}
	os.Exit(code)
}
