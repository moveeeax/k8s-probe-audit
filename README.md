# k8s-probe-audit

[![CI](https://github.com/moveeeax/k8s-probe-audit/actions/workflows/ci.yml/badge.svg)](https://github.com/moveeeax/k8s-probe-audit/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.22-00ADD8?logo=go)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

Flag Kubernetes workloads that are missing a **liveness** or **readiness** probe, and
fail CI before a probe-less pod ships.

A container with no liveness probe never gets restarted when its process wedges; a
container with no readiness probe takes traffic before it is ready. Both are optional
in the API, so they get skipped under deadline pressure and the cost shows up later, in
production. `k8s-probe-audit` is a cheap gate that catches that at PR time.

## How it works

- Walks every path you give it — files and directories, recursively — and reads
  `*.yaml` / `*.yml` (explicit file paths are read whatever their extension).
- Splits multi-document manifests on `---` boundaries and decodes each document.
- For every `Deployment`, `StatefulSet`, `DaemonSet` and `ReplicaSet`, inspects each
  container in the pod template. Non-workload kinds are skipped silently.
- Reports the `kind`, `namespace`, `name`, `container` and which probe is missing.
- Exits **1** when anything is flagged, so a CI step fails the build.

## Install

```bash
go install github.com/moveeeax/k8s-probe-audit@latest
```

Or build from source:

```bash
git clone https://github.com/moveeeax/k8s-probe-audit
cd k8s-probe-audit
go build -o k8s-probe-audit .
```

## Usage

```
k8s-probe-audit scan [flags] <path>...

  --require both|liveness|readiness   which probes a container must declare (default both)
  --json                              emit findings as a JSON array
```

Scan a directory of manifests:

```console
$ k8s-probe-audit scan ./examples/manifests
KIND        NAMESPACE  NAME      CONTAINER  MISSING
Deployment  shop       checkout  checkout   liveness
Deployment  shop       checkout  checkout   readiness
Deployment  shop       checkout  sidecar    liveness

3 probe(s) missing
$ echo $?
1
```

Only require readiness (e.g. for a batch tier where liveness is deliberately omitted):

```console
$ k8s-probe-audit scan --require readiness ./examples/manifests
```

Machine-readable output for a dashboard or a PR annotator:

```console
$ k8s-probe-audit scan --json ./examples/manifests
[
  {
    "kind": "Deployment",
    "name": "checkout",
    "namespace": "shop",
    "container": "checkout",
    "missing": "liveness",
    "source": "examples/manifests/deployment-missing-probes.yaml"
  }
]
```

A clean tree exits 0:

```console
$ k8s-probe-audit scan ./examples/manifests/statefulset-ok.yaml
OK: every workload container declares the required probes
$ echo $?
0
```

## In CI

```yaml
- name: Audit probes
  run: |
    go run github.com/moveeeax/k8s-probe-audit@latest scan ./manifests
```

The non-zero exit on any finding fails the job — no extra assertion needed.

## Development

```bash
go test ./...      # unit tests over fixture manifests in internal/audit/testdata
go vet ./...
```

## License

MIT — see [LICENSE](LICENSE).
