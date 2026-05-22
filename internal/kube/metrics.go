package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// ContainerMetrics holds live CPU and memory usage for a container.
type ContainerMetrics struct {
	Name     string
	CPUUsage resource.Quantity
	MemUsage resource.Quantity
}

// PodMetrics holds live usage for all containers in a pod.
type PodMetrics struct {
	Name       string
	Containers []ContainerMetrics
}

// FetchMetrics returns live usage from the metrics-server API, keyed by pod name.
// Returns nil without error when metrics-server is unavailable.
func (c *Client) FetchMetrics(namespace string) (map[string]PodMetrics, error) {
	list, err := c.metrics.MetricsV1beta1().PodMetricses(namespace).List(
		context.Background(), metav1.ListOptions{},
	)
	if err != nil {
		if isMetricsUnavailable(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("fetch metrics in %q: %w", namespace, err)
	}

	result := make(map[string]PodMetrics, len(list.Items))
	for _, pm := range list.Items {
		result[pm.Namespace+"/"+pm.Name] = podMetricsFromAPI(pm)
	}
	return result, nil
}

func podMetricsFromAPI(pm metricsv1beta1.PodMetrics) PodMetrics {
	containers := make([]ContainerMetrics, 0, len(pm.Containers))
	for _, c := range pm.Containers {
		containers = append(containers, ContainerMetrics{
			Name:     c.Name,
			CPUUsage: quantityOrZero(c.Usage, corev1.ResourceCPU),
			MemUsage: quantityOrZero(c.Usage, corev1.ResourceMemory),
		})
	}
	return PodMetrics{Name: pm.Name, Containers: containers}
}

// isMetricsUnavailable returns true for errors that indicate the metrics-server
// API is not installed or not yet ready, so callers can show N/A gracefully.
func isMetricsUnavailable(err error) bool {
	return apierrors.IsNotFound(err) ||
		apierrors.IsServiceUnavailable(err) ||
		apierrors.IsTimeout(err)
}
