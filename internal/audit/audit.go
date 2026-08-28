// Package audit scans Kubernetes manifests for containers that are missing
// a liveness or readiness probe, so a probe-less workload can be caught in CI.
package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"
)

// Require selects which probes a container must declare.
type Require string

const (
	RequireBoth      Require = "both"
	RequireLiveness  Require = "liveness"
	RequireReadiness Require = "readiness"
)

// ParseRequire validates the --require value.
func ParseRequire(s string) (Require, error) {
	switch Require(s) {
	case RequireBoth, RequireLiveness, RequireReadiness:
		return Require(s), nil
	default:
		return "", fmt.Errorf("invalid --require %q (want both, liveness or readiness)", s)
	}
}

// wantsLiveness reports whether the mode requires a liveness probe.
func (r Require) wantsLiveness() bool { return r == RequireBoth || r == RequireLiveness }

// wantsReadiness reports whether the mode requires a readiness probe.
func (r Require) wantsReadiness() bool { return r == RequireBoth || r == RequireReadiness }

// workloadKinds are the kinds whose pod template we inspect.
var workloadKinds = map[string]bool{
	"Deployment":  true,
	"StatefulSet": true,
	"DaemonSet":   true,
	"ReplicaSet":  true,
}

// Finding is a single missing-probe violation.
type Finding struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Missing   string `json:"missing"` // "liveness" or "readiness"
	Source    string `json:"source"`  // file the manifest came from
}

// manifest is the minimal shape we decode out of each YAML document.
type manifest struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		Template struct {
			Spec struct {
				Containers []container `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
}

type container struct {
	Name           string          `json:"name"`
	LivenessProbe  json.RawMessage `json:"livenessProbe"`
	ReadinessProbe json.RawMessage `json:"readinessProbe"`
}

// present reports whether a probe field holds a real object (not absent/null).
func present(raw json.RawMessage) bool {
	t := strings.TrimSpace(string(raw))
	return t != "" && t != "null"
}

// Scan walks every path (file or directory, recursively) and returns the
// findings sorted for stable output. Directories are searched for *.yaml and
// *.yml files; explicit file paths are read regardless of extension.
func Scan(paths []string, req Require) ([]Finding, error) {
	var files []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			err := filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				if ext := strings.ToLower(filepath.Ext(path)); ext == ".yaml" || ext == ".yml" {
					files = append(files, path)
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
		} else {
			files = append(files, p)
		}
	}

	var findings []Finding
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		fs, err := scanBytes(data, req, f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		findings = append(findings, fs...)
	}

	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Container != b.Container {
			return a.Container < b.Container
		}
		return a.Missing < b.Missing
	})
	return findings, nil
}

// scanBytes parses a possibly multi-document manifest and returns its findings.
func scanBytes(data []byte, req Require, source string) ([]Finding, error) {
	var out []Finding
	for _, doc := range splitDocuments(data) {
		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}
		var m manifest
		if err := yaml.Unmarshal(doc, &m); err != nil {
			return nil, err
		}
		if !workloadKinds[m.Kind] {
			continue
		}
		ns := m.Metadata.Namespace
		if ns == "" {
			ns = "default"
		}
		for _, c := range m.Spec.Template.Spec.Containers {
			if req.wantsLiveness() && !present(c.LivenessProbe) {
				out = append(out, Finding{m.Kind, m.Metadata.Name, ns, c.Name, "liveness", source})
			}
			if req.wantsReadiness() && !present(c.ReadinessProbe) {
				out = append(out, Finding{m.Kind, m.Metadata.Name, ns, c.Name, "readiness", source})
			}
		}
	}
	return out, nil
}

// splitDocuments splits a YAML stream on `---` document separators that sit on
// their own line, the boundary used by every multi-document manifest.
func splitDocuments(data []byte) [][]byte {
	lines := bytes.Split(data, []byte("\n"))
	var docs [][]byte
	var cur [][]byte
	flush := func() {
		docs = append(docs, bytes.Join(cur, []byte("\n")))
		cur = nil
	}
	for _, ln := range lines {
		if isSeparator(ln) {
			flush()
			continue
		}
		cur = append(cur, ln)
	}
	flush()
	return docs
}

// isSeparator reports whether a line is a bare `---` document boundary.
func isSeparator(line []byte) bool {
	t := strings.TrimRight(string(line), " \t\r")
	return t == "---"
}
