package kube

import (
	"fmt"

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
