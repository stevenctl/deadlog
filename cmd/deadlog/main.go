// Command deadlog analyzes lock logs for deadlock detection.
package main

import (
	"fmt"
	"os"

	"github.com/stevenctl/deadlog/analyze"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "analyze":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: deadlog analyze <file>")
			fmt.Fprintln(os.Stderr, "       deadlog analyze -  (read from stdin)")
			os.Exit(1)
		}
		runAnalyze(os.Args[2])
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("deadlog - Debug Go mutex deadlocks")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  deadlog analyze <file>   Analyze a log file for deadlocks")
	fmt.Println("  deadlog analyze -        Read from stdin")
	fmt.Println("  deadlog help             Show this help")
	fmt.Println()
	fmt.Println("Example:")
	fmt.Println("  go run ./myapp 2>&1 | deadlog analyze -")
}

func runAnalyze(path string) {
	var result *analyze.Result
	var err error

	if path == "-" {
		result, err = analyze.Analyze(os.Stdin)
	} else {
		result, err = analyze.AnalyzeFile(path)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	analyze.PrintReport(os.Stdout, result)

	// Exit with non-zero if issues found
	if len(result.Stuck) > 0 || len(result.Held) > 0 {
		os.Exit(1)
	}
}
