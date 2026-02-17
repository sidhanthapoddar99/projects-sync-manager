package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sid/psm/internal/tui"
)

var version = "dev"

func main() {
	dir := flag.String("d", ".", "Target directory to scan")
	depth := flag.Int("h", 3, "Max depth for git repo discovery")
	ver := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *ver {
		fmt.Println("psm version", version)
		os.Exit(0)
	}

	// Resolve directory
	targetDir := *dir
	if targetDir == "." {
		var err error
		targetDir, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot get current directory: %v\n", err)
			os.Exit(1)
		}
	}

	// Verify directory exists
	info, err := os.Stat(targetDir)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: %s is not a valid directory\n", targetDir)
		os.Exit(1)
	}

	if err := tui.Run(targetDir, *depth); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
