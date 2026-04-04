package main

import (
	"fmt"
	"os"

	"github.com/calebfaruki/impromptu/internal/contentcheck"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: impromptu <command> [args]\n")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "check":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "usage: impromptu check <directory>\n")
			os.Exit(1)
		}
		runCheck(os.Args[2])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runCheck(dir string) {
	violations, err := contentcheck.CheckDirectory(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	if len(violations) == 0 {
		fmt.Println("PASS")
		return
	}
	for _, v := range violations {
		fmt.Println(v.Error())
	}
	os.Exit(1)
}
