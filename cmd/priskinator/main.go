// Command priskinator evaluates pull requests and decides when a human must review them.
package main

import "os"

// main only wires the process to run, which holds the logic and the tests.
func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
