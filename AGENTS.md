# AGENTS.md — podres

This file provides context for AI coding agents (Claude Code, GitHub Copilot, etc.) working on the `podres` project.

---

## Project Overview

`podres` is an open-source `kubectl` plugin written in Go. It provides a real-time, colorized, watch-mode view of Kubernetes pod and container resource requests, limits, and live utilization — all in a single compact terminal output.

It is installable as a standalone binary or as a kubectl plugin (`kubectl podres`) via Krew.

---

## Goals

- Display running pods in a namespace with per-container resource breakdowns
- Show CPU and memory: requests, limits, live usage, and usage percentage
- Colorize output based on usage thresholds (green / yellow / red)
- Highlight warnings for containers approaching or exceeding resource limits
- Highlight pods with non-zero restart counts
- Support watch mode (continuous refresh loop)
- Work against any Kubernetes or OpenShift cluster via kubeconfig or in-cluster auth
- Cross-platform: Linux, macOS, Windows (amd64 and arm64)

---

## Tech Stack

| Concern | Library / Tool |
|---|---|
| Language | Go 1.22+ |
| Kubernetes API | `k8s.io/client-go` |
| Metrics API | `k8s.io/metrics` |
| Terminal styling | `github.com/charmbracelet/lipgloss` |
| TUI / watch loop | `github.com/charmbracelet/bubbletea` |
| Table rendering | `github.com/charmbracelet/bubbles/table` or manual lipgloss layout |
| CLI flags | `github.com/spf13/cobra` |
| Build/release | GoReleaser |

---

## Project Structure

```text
podres/
├── cmd/
│   └── root.go          # Cobra root command, flag definitions
├── internal/
│   ├── kube/
│   │   ├── client.go    # kubeconfig + in-cluster client setup
│   │   ├── pods.go      # List pods and container specs (requests/limits)
│   │   └── metrics.go   # Fetch live usage from metrics-server
│   ├── ui/
│   │   ├── model.go     # Bubbletea model (state, update, view)
│   │   ├── table.go     # Table layout and row rendering
│   │   └── styles.go    # Lipgloss color themes and threshold styles
│   └── threshold/
│       └── threshold.go # Threshold logic and warning classification
├── main.go
├── go.mod
├── go.sum
├── AGENTS.md
├── README.md
└── LICENSE              # MIT
```

---

## Color Thresholds

Apply to both CPU% and MEM% columns:

| Usage      | Style    |
|------------|----------|
| < 75%      | Green    |
| 75% to 94% | Yellow   |
| >= 95%     | Bold Red |

Restart count > 0 should highlight the pod name row in yellow.

---

## Output Format

Output is a table scoped to a single namespace. Each pod is a group header row, with one row per container beneath it. Example:

```text
NAMESPACE: api-gateway                      Refreshed: 01:47:57   TZ: America/New_York

POD                                    CONTAINER     CPU-REQ  CPU-LIM  CPU-USE  CPU%   MEM-REQ  MEM-LIM  MEM-USE  MEM%
api-gateway-fd64cf66d-gh76r            apisix        250m     500m     947m     1%     512Mi    1Gi      536Mi    52%
                                       redis         100m     200m     0m       0%     128Mi    256Mi    9Mi      3%
                                       istio-proxy   100m     200m     158m     0%     200Mi    400Mi    77Mi     19%
api-gateway-rate-limit-redis-7994b..   redis         100m     200m     0m       0%     128Mi    256Mi    9Mi      3%
                                       istio-proxy   100m     200m     ...

⚠  Warnings:
   apisix (api-gateway-fd64cf66d-gh76r): MEM 52%  — approaching threshold
```

---

## CLI Flags

| Flag | Default | Description |
|---|---|---|
| `-n, --namespace` | current context namespace | Namespace to watch |
| `--interval` | `5s` | Refresh interval in watch mode |
| `--no-watch` | false | Print once and exit |
| `--kubeconfig` | `~/.kube/config` | Path to kubeconfig |
| `--context` | current context | Kubeconfig context to use |
| `--threshold-warn` | `75` | Yellow warning threshold (percent) |
| `--threshold-crit` | `95` | Red critical threshold (percent) |
| `--no-color` | false | Disable colorized output |

---

## Key Implementation Notes

- Use `client-go` dynamic informers or direct REST calls — do not shell out to `kubectl`
- Metrics come from the `metrics.k8s.io/v1beta1` API (requires metrics-server installed on the cluster)
- Gracefully handle missing metrics-server (show `N/A` instead of crashing)
- Percentage is calculated as: `(usage / limit) * 100` — fall back to `(usage / request) * 100` if no limit is set
- Sidecar containers (name contains `istio-proxy`, `envoy`, or `linkerd-proxy`) may be visually dimmed — consider a `--dim-sidecars` flag
- In-cluster auth should work automatically via `rest.InClusterConfig()`
- The plugin must be named `kubectl-podres` on disk for kubectl plugin discovery to work

---

## Build

```bash
go build -o kubectl-podres ./main.go
```

Cross-platform:

```bash
GOOS=linux   GOARCH=amd64 go build -o dist/kubectl-podres-linux-amd64 ./main.go
GOOS=darwin  GOARCH=arm64 go build -o dist/kubectl-podres-darwin-arm64 ./main.go
GOOS=windows GOARCH=amd64 go build -o dist/kubectl-podres-windows-amd64.exe ./main.go
```

---

## Out of Scope (for now)

- Multi-namespace or all-namespace views
- Node-level resource aggregation
- Prometheus/custom metrics support
- Interactive TUI navigation (k9s-style drill down)
