package main

import (
	"fmt"
	"io"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `usage: priskinator <command>

Commands:
  evaluate  Evaluate a change and print its verdict
  version   Print the priskinator version

Exit codes:
  0  the command ran; for evaluate, a verdict was computed, whatever its risk
  1  evaluation failed, and the verdict printed is an evaluation error
  2  usage error
`

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return 2
	}

	switch args[0] {
	case "evaluate":
		return runEvaluate(args[1:], stdout, stderr)
	case "version":
		_, _ = fmt.Fprintf(stdout, "priskinator %s\n", version)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}
