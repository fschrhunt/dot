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
		if e := only(command, args, "--settled"); e != nil {
			return 2, e
		}
		return dotsync.Run(paths, slices.Contains(args, "--settled"), stderr)
	case "add":
		return app.Add(paths, args, out, stderr)
	}
	c, e := config.Load(paths)
	if e != nil {
		return 2, e
	}
	switch command {
	case "status":
		return app.Status(c, first, out)
	case "forget":
		return app.Forget(c, args, out)
	case "agents":
		return app.Agents(c, out)
	case "log":
		return app.Log(c, first, out)
	case "undo":
		if len(args) != 1 {
			return 2, setup.Fail("usage: dot undo <live-path>")
		}
		return app.Undo(c, first, out, stderr)
	case "timer", "install":
		if e := only(command, args, "--remove"); e != nil {
			return 2, e
		}
		return schedule.Run(paths, c.Sync, slices.Contains(args, "--remove"), out)
	case "take":
		if len(args) != 1 {
			return 2, setup.Fail("usage: dot take <live-path>")
		}
		return app.Take(c, first, out)
	case "apply":
		if e := only(command, args, "-n", "--force"); e != nil {
			return 2, e
		}
		return app.Apply(c, slices.Contains(args, "-n"), slices.Contains(args, "--force"), out, stderr)
	}
	return 2, setup.Fail("unknown command " + command + " (see dot help)")
}

// only rejects an argument a command does not take, naming the first in sorted order, so a
// mistyped flag never runs the command without it.
func only(command string, args []string, allowed ...string) error {
	unknown := slices.DeleteFunc(slices.Clone(args), func(s string) bool { return slices.Contains(allowed, s) })
	if len(unknown) == 0 {
		return nil
	}
	slices.Sort(unknown)
	usage := "dot " + command
	for _, flag := range allowed {
		usage += " [" + flag + "]"
	}
	return setup.Fail("unknown option " + unknown[0] + " (" + usage + ")")
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
