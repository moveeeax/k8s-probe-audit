// Command k8s-probe-audit scans Kubernetes manifests and flags workload
// containers that are missing a liveness or readiness probe, exiting non-zero
// when it finds one so it can gate CI.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/moveeeax/k8s-probe-audit/internal/audit"
)

const usage = `k8s-probe-audit — flag workloads missing liveness/readiness probes

Usage:
  k8s-probe-audit scan [flags] <path>...

Flags:
  --require both|liveness|readiness   which probes a container must declare (default both)
  --json                              emit findings as a JSON array
  -h, --help                          show this help

Exit status is 1 when any probe is missing, so the command gates CI.`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Println(usage)
		return 0
	}
	if args[0] != "scan" {
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s\n", args[0], usage)
		return 2
	}

	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, usage) }
	requireFlag := fs.String("require", "both", "which probes to require: both|liveness|readiness")
	jsonOut := fs.Bool("json", false, "emit findings as a JSON array")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	req, err := audit.ParseRequire(*requireFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}

	paths := fs.Args()
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "error: at least one path is required")
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}

	findings, err := audit.Scan(paths, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}

	if *jsonOut {
		err = audit.WriteJSON(os.Stdout, findings)
	} else {
		err = audit.WriteTable(os.Stdout, findings)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}

	if len(findings) > 0 {
		return 1
	}
	return 0
}
