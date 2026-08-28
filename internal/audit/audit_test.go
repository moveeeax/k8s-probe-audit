package audit

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestParseRequire(t *testing.T) {
	for _, s := range []string{"both", "liveness", "readiness"} {
		if _, err := ParseRequire(s); err != nil {
			t.Errorf("ParseRequire(%q) unexpected error: %v", s, err)
		}
	}
	if _, err := ParseRequire("startup"); err == nil {
		t.Error("ParseRequire(\"startup\") should fail")
	}
}

func TestPresent(t *testing.T) {
	cases := map[string]bool{
		`{"httpGet":{"path":"/"}}`: true,
		`{}`:                       true,
		``:                         false,
		`null`:                     false,
		`  null `:                  false,
	}
	for raw, want := range cases {
		if got := present(json.RawMessage(raw)); got != want {
			t.Errorf("present(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestScanBytes(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		req     Require
		want    []Finding // Source is filled by the caller's expectation
		wantErr bool
	}{
		{
			name: "both probes present is clean",
			file: "clean.yaml",
			req:  RequireBoth,
			want: nil,
		},
		{
			name: "missing readiness flagged as one finding",
			file: "missing-readiness.yaml",
			req:  RequireBoth,
			want: []Finding{
				{Kind: "Deployment", Name: "api", Namespace: "default", Container: "app", Missing: "readiness"},
			},
		},
		{
			name: "missing both probes yields two findings per container",
			file: "missing-both.yaml",
			req:  RequireBoth,
			want: []Finding{
				{Kind: "StatefulSet", Name: "db", Namespace: "data", Container: "db", Missing: "liveness"},
				{Kind: "StatefulSet", Name: "db", Namespace: "data", Container: "db", Missing: "readiness"},
			},
		},
		{
			name: "require liveness ignores a missing readiness",
			file: "missing-readiness.yaml",
			req:  RequireLiveness,
			want: nil,
		},
		{
			name: "multi-doc skips non-workload kinds and reports the workload",
			file: "multi-doc.yaml",
			req:  RequireBoth,
			want: []Finding{
				{Kind: "DaemonSet", Name: "agent", Namespace: "kube-system", Container: "agent", Missing: "liveness"},
			},
		},
		{
			name:    "malformed yaml errors",
			file:    "malformed.txt",
			req:     RequireBoth,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("testdata", tc.file)
			got, err := Scan([]string{path}, tc.req)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Scan error: %v", err)
			}
			// Fill the expected Source with the fixture path for comparison.
			want := make([]Finding, len(tc.want))
			for i := range tc.want {
				want[i] = tc.want[i]
				want[i].Source = path
			}
			if !equalFindings(got, want) {
				t.Errorf("findings mismatch\n got: %+v\nwant: %+v", got, want)
			}
		})
	}
}

func TestScanDirectory(t *testing.T) {
	got, err := Scan([]string{"testdata"}, RequireReadiness)
	if err != nil {
		t.Fatalf("Scan(dir) error: %v", err)
	}
	// Under --require readiness, only readiness gaps count.
	for _, f := range got {
		if f.Missing != "readiness" {
			t.Errorf("unexpected non-readiness finding under RequireReadiness: %+v", f)
		}
	}
	if len(got) == 0 {
		t.Error("expected at least one readiness finding across the fixtures")
	}
}

func TestWriteJSONAlwaysArray(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if got := bytes.TrimSpace(buf.Bytes()); string(got) != "[]" {
		t.Errorf("WriteJSON(nil) = %q, want []", got)
	}
}

func equalFindings(a, b []Finding) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
