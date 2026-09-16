package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

type GlobalConfig struct {
	Verbose bool
}

func main() {
	rootFlags := flag.NewFlagSet("hackernewscli", flag.ExitOnError)
	verbose := rootFlags.Bool("verbose", false, "enable verbose output")

	rootFlags.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: hackernewscli [global flags]  [subcommand flags]\n\n")
		fmt.Fprintln(os.Stderr, "Global Flags:")
		rootFlags.PrintDefaults()
		fmt.Fprintln(os.Stderr, "\nSubcommands:")
		fmt.Fprintln(os.Stderr, "		fetch: Fetch HackerNews")
	}

	if len(os.Args) < 2 {
		rootFlags.Usage()
		os.Exit(1)
	}

	err := rootFlags.Parse(os.Args[1:])
	if err != nil {
		os.Exit(1)
	}

	cfg := GlobalConfig{
		Verbose: *verbose,
	}

	args := rootFlags.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: subcommand required.")
		os.Exit(1)
	}

	subcommand := args[0]
	subArgs := args[1:]

	var runErr error
	switch subcommand {
	case "fetch":
		runErr = handleFetchCmd(cfg, subArgs)
	case "help":
		rootFlags.Usage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %q\n\n", subcommand)
		rootFlags.Usage()
		os.Exit(1)
	}

	if runErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", runErr)
		os.Exit(1)
	}
}

type FetchConfig struct {
	Limit int
	Type  string
}

func handleFetchCmd(cfg Config, args []string) error {
	fetchFlags := flag.NewFlagSet("fetch", flag.ContinueOnError)

	limit := fetchFlags.Int("limit", 10, "Number of stories to fetch")
	storyType := fetchFlags.String("type", "top", "Story type (top, new, best)")

	fetchFlags.Usage = func() {
		fmt.Fprintln(os.Stdout, "Usage: hackernewscli fetch [flags]")
		fmt.Fprintln(os.Stdout, "\nFlags:")
		fetchFlags.PrintDefaults()
	}

	if err := fetchFlags.Parse(args); err != nil {
		return err
	}

	if cfg.Verbose {
		fmt.Println("[DEBUG] Running fetch command with verbose mode active")
	}

	return nil
}
