#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-podres-demo}"
NAMESPACE="${NAMESPACE:-podres-demo}"
IMAGE="${IMAGE:-nginx:1.25}"

# ── helpers ──────────────────────────────────────────────────────────────────

info()  { echo "▶  $*"; }
ok()    { echo "✔  $*"; }
die()   { echo "✘  $*" >&2; exit 1; }

require() {
  for cmd in "$@"; do
    command -v "$cmd" &>/dev/null || die "'$cmd' is not installed or not in PATH"
  done
}

# ── preflight ─────────────────────────────────────────────────────────────────

require kind kubectl

# ── cluster ──────────────────────────────────────────────────────────────────

if kind get clusters 2>/dev/null | grep -qx "$CLUSTER_NAME"; then
  info "kind cluster '$CLUSTER_NAME' already exists — skipping creation"
  # Refresh the kubeconfig entry; 'kind create cluster' normally does this as a
  # side effect, so we must do it explicitly when skipping creation to ensure
  # the context exists before any kubectl calls below.
  kind export kubeconfig --name "$CLUSTER_NAME"
else
  info "Creating kind cluster '$CLUSTER_NAME' …"
  kind create cluster --name "$CLUSTER_NAME" --wait 60s
  ok "Cluster ready"
fi

KUBECONTEXT="kind-${CLUSTER_NAME}"
KUBECTL="kubectl --context=$KUBECONTEXT"

# ── namespace + ResourceQuota ─────────────────────────────────────────────────

info "Applying namespace '$NAMESPACE' and ResourceQuota …"

$KUBECTL apply -f - <<EOF
---
apiVersion: v1
kind: Namespace
metadata:
  name: $NAMESPACE
  labels:
    managed-by: podres-demo-script
---
apiVersion: v1
kind: ResourceQuota
metadata:
  name: demo-quota
  namespace: $NAMESPACE
spec:
  hard:
    pods: "20"
    requests.cpu: "2"
    requests.memory: 2Gi
    limits.cpu: "4"
    limits.memory: 4Gi
EOF

ok "Namespace and ResourceQuota applied"

# ── deployment ────────────────────────────────────────────────────────────────

info "Applying demo Deployment …"

$KUBECTL apply -f - <<EOF
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo-app
  namespace: $NAMESPACE
  labels:
    app: demo-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: demo-app
  template:
    metadata:
      labels:
        app: demo-app
    spec:
      containers:
        - name: web
          image: $IMAGE
          ports:
            - containerPort: 80
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 200m
              memory: 128Mi
        - name: sidecar
          image: busybox:1.36
          command: ["sh", "-c", "while true; do sleep 30; done"]
          resources:
            requests:
              cpu: 10m
              memory: 16Mi
            limits:
              cpu: 50m
              memory: 32Mi
EOF

ok "Deployment applied"

# ── wait for rollout ──────────────────────────────────────────────────────────

info "Waiting for rollout …"
$KUBECTL rollout status deployment/demo-app -n "$NAMESPACE" --timeout=120s
ok "Rollout complete"

# ── summary ───────────────────────────────────────────────────────────────────

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Demo cluster ready!"
echo "  Cluster   : $CLUSTER_NAME"
echo "  Namespace : $NAMESPACE"
echo "  Context   : $KUBECONTEXT"
echo ""
echo "  Run podres:"
echo "    kubectl podres -n $NAMESPACE --context $KUBECONTEXT"
echo ""
echo "  Tear down:"
echo "    kind delete cluster --name $CLUSTER_NAME"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
