package kube

import (
	"fmt"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Client holds the Kubernetes and metrics API clients.
type Client struct {
	kube       kubernetes.Interface
	metrics    metrics.Interface
	clientCfg  clientcmd.ClientConfig
}

// New returns a Client configured from kubeconfig or in-cluster auth.
// kubeconfig and context may be empty strings to use defaults.
func New(kubeconfig, context string) (*Client, error) {
	clientCfg := buildClientConfig(kubeconfig, context)

	cfg, err := clientCfg.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build kubeconfig: %w", err)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}

	metricsClient, err := metrics.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create metrics client: %w", err)
	}

	return &Client{kube: kubeClient, metrics: metricsClient, clientCfg: clientCfg}, nil
}

// ClusterInfo returns the cluster name and auth user from the active kubeconfig context.
// It handles both standard kubeconfigs and OpenShift-style ones where auth info is
// stored as "user/server" — in that case only the first segment is returned as the user.
func (c *Client) ClusterInfo() (cluster, user string, err error) {
	raw, err := c.clientCfg.RawConfig()
	if err != nil {
		return "", "", fmt.Errorf("raw config: %w", err)
	}
	if ctx, ok := raw.Contexts[raw.CurrentContext]; ok {
		cluster = ctx.Cluster
		user = firstSegment(ctx.AuthInfo)
	}
	return cluster, user, nil
}

// firstSegment returns the portion of s before the first '/', or s itself if there is none.
func firstSegment(s string) string {
	before, _, _ := strings.Cut(s, "/")
	return before
}

// CurrentNamespace returns the namespace from the active kubeconfig context.
func (c *Client) CurrentNamespace() (string, error) {
	ns, _, err := c.clientCfg.Namespace()
	if err != nil {
		return "", fmt.Errorf("resolve namespace: %w", err)
	}
	if ns == "" {
		return "default", nil
	}
	return ns, nil
}

// buildClientConfig constructs a ClientConfig honoring (in priority order):
//  1. --kubeconfig flag (explicit path)
//  2. $KUBECONFIG env var (colon-separated list)
//  3. ~/.kube/config
//  4. in-cluster config (when running inside a pod)
func buildClientConfig(kubeconfigPath, context string) clientcmd.ClientConfig {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		loadingRules.ExplicitPath = kubeconfigPath
	}
	overrides := &clientcmd.ConfigOverrides{}
	if context != "" {
		overrides.CurrentContext = context
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
}
