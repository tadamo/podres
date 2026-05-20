# podres

A `kubectl` plugin that shows a real-time, colorized view of Kubernetes pod and container resource requests, limits, and live utilization in a single compact table.

```
NAMESPACE: api-gateway   CLUSTER: prod-eks   USER: tom   Refreshed: 14:32:07 EDT

 POD                              PHASE    CPU-REQ  CPU-LIM  CPU-USE  CPU%   MEM-REQ  MEM-LIM  MEM-USE  MEM%
 api-gw-fd64cf66d-gh76r           Running
                                  apisix   250m     500m     471m     94%    512Mi    1Gi      536Mi    52%
                                  redis    100m     200m     0m       0%     128Mi    256Mi    9Mi      3%
 api-gw-rate-limit-redis-7994b..  Running
                                  redis    100m     200m     0m       0%     128Mi    256Mi    9Mi      3%

 TOTAL                                     450m     900m     471m            768Mi    1.5Gi    545Mi

⚠  Warnings:
   apisix (api-gw-fd64cf66d-gh76r): CPU 94% — approaching threshold
```

## Prerequisites

- A running Kubernetes cluster
- [metrics-server](https://github.com/kubernetes-sigs/metrics-server) installed in the cluster (for live CPU/memory usage)
- `kubectl` configured with a valid kubeconfig

## Installation

### kubectl krew (recommended)

```bash
kubectl krew install podres
```

### Direct download

Download the binary for your platform from the [releases page](https://github.com/tadamo/podres/releases), rename it to `kubectl-podres`, and place it somewhere on your `PATH`.

```bash
# macOS (Apple Silicon)
curl -L https://github.com/tadamo/podres/releases/latest/download/kubectl-podres_darwin_arm64.tar.gz | tar xz
sudo mv kubectl-podres /usr/local/bin/kubectl-podres

# Linux (amd64)
curl -L https://github.com/tadamo/podres/releases/latest/download/kubectl-podres_linux_amd64.tar.gz | tar xz
sudo mv kubectl-podres /usr/local/bin/kubectl-podres
```

### Build from source

```bash
go install github.com/tadamo/podres@latest
```

Or clone and build manually:

```bash
git clone https://github.com/tadamo/podres.git
cd podres
go build -o kubectl-podres ./main.go
sudo mv kubectl-podres /usr/local/bin/kubectl-podres
```

## Usage

```bash
# Watch the current namespace (refreshes every 5s)
kubectl podres

# Watch a specific namespace
kubectl podres -n kube-system

# Filter by label selector
kubectl podres -l app=nginx

# One-shot snapshot (no watch mode)
kubectl podres --no-watch

# Custom refresh interval
kubectl podres --interval 10s

# Show full pod/container names without truncation
kubectl podres --wide

# Start sorted by CPU usage (descending)
kubectl podres --sort cpu
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `-n, --namespace` | current context | Namespace to watch |
| `-l, --selector` | | Label selector to filter pods (e.g. `app=nginx`) |
| `--interval` | `5s` | Refresh interval in watch mode |
| `--no-watch` | `false` | Print once and exit |
| `--kubeconfig` | `~/.kube/config` | Path to kubeconfig |
| `--context` | current context | Kubeconfig context to use |
| `--threshold-warn` | `75` | Yellow warning threshold (percent) |
| `--threshold-crit` | `95` | Red critical threshold (percent) |
| `--no-color` | `false` | Disable colorized output |
| `--pod-dividers` | `false` | Draw a horizontal rule between each pod |
| `-w, --wide` | `false` | Show full pod and container names without truncation |
| `--sort` | | Initial sort column: `cpu`, `mem`, `restarts`, `name` |

## Keyboard shortcuts (watch mode)

| Key | Action |
|---|---|
| `c` | Sort by CPU% |
| `m` | Sort by Memory% |
| `r` | Sort by restart count |
| `n` | Sort by pod name |
| `0` | Clear sort |
| `↑` / `↓` | Scroll |
| `PgUp` / `PgDn` | Scroll by page |
| `q` / `Ctrl+C` | Quit |

## Color coding

| Usage | Color |
|---|---|
| < 75% | Green |
| 75–94% | Yellow |
| ≥ 95% | Bold red |

Pods with non-zero restart counts are highlighted in yellow. The last termination reason (OOMKilled, Error, etc.) is shown in the RESTARTS column.

## License

MIT — see [LICENSE](LICENSE)
