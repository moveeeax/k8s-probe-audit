package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
)

// WriteTable prints findings as an aligned table. When there are none it prints
// a single OK line so a passing run is still legible.
func WriteTable(w io.Writer, findings []Finding) error {
	if len(findings) == 0 {
		_, err := fmt.Fprintln(w, "OK: every workload container declares the required probes")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "KIND\tNAMESPACE\tNAME\tCONTAINER\tMISSING")
	for _, f := range findings {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", f.Kind, f.Namespace, f.Name, f.Container, f.Missing)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "\n%d probe(s) missing\n", len(findings))
	return err
}

// WriteJSON prints findings as a machine-readable array (always an array, never
// null, so downstream tooling can rely on the shape).
func WriteJSON(w io.Writer, findings []Finding) error {
	if findings == nil {
		findings = []Finding{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(findings)
}
