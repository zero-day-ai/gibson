package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/zero-day-ai/gibson/cmd/gibson/internal"
)

func main() {
	// Set up panic recovery to handle unexpected errors gracefully
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "Fatal error: %v\n", r)
			if internal.IsVerbose() {
				fmt.Fprintf(os.Stderr, "Stack trace:\n%s\n", debug.Stack())
			} else {
				fmt.Fprintln(os.Stderr, "Run with --verbose for stack trace")
			}
			os.Exit(internal.ExitError)
		}
	}()

	// Create base context
	ctx := context.Background()

	// Execute root command
	// Execute() now handles signal notification internally via signal.NotifyContext
	if err := Execute(ctx); err != nil {
		// Handle the error and get the appropriate exit code
		exitCode := internal.HandleError(rootCmd, err)
		os.Exit(exitCode)
	}

	// Successful execution
	os.Exit(internal.ExitSuccess)
}
