package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespaceQuota holds the aggregate hard limits from all ResourceQuota objects in a namespace.
type NamespaceQuota struct {
	CPURequest resource.Quantity
	CPULimit   resource.Quantity
	MemRequest resource.Quantity
	MemLimit   resource.Quantity
}

// GetResourceQuota returns the aggregate hard limits across all ResourceQuota objects in
// the namespace, or nil if none exist.
func (c *Client) GetResourceQuota(namespace string) (*NamespaceQuota, error) {
	list, err := c.kube.CoreV1().ResourceQuotas(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list resource quotas in %q: %w", namespace, err)
	}
	if len(list.Items) == 0 {
		return nil, nil
	}

	q := &NamespaceQuota{}
	for _, rq := range list.Items {
		addHard(q, rq.Spec.Hard)
	}
	return q, nil
}

// addHard accumulates hard limits from one ResourceQuota into q.
// When multiple quotas define the same resource the stricter (lower) value wins.
func addHard(q *NamespaceQuota, hard corev1.ResourceList) {
	if v, ok := hard[corev1.ResourceRequestsCPU]; ok {
		if q.CPURequest.IsZero() || v.Cmp(q.CPURequest) < 0 {
			q.CPURequest = v.DeepCopy()
		}
	}
	if v, ok := hard[corev1.ResourceLimitsCPU]; ok {
		if q.CPULimit.IsZero() || v.Cmp(q.CPULimit) < 0 {
			q.CPULimit = v.DeepCopy()
		}
	}
	if v, ok := hard[corev1.ResourceRequestsMemory]; ok {
		if q.MemRequest.IsZero() || v.Cmp(q.MemRequest) < 0 {
			q.MemRequest = v.DeepCopy()
		}
	}
	if v, ok := hard[corev1.ResourceLimitsMemory]; ok {
		if q.MemLimit.IsZero() || v.Cmp(q.MemLimit) < 0 {
			q.MemLimit = v.DeepCopy()
		}
	}
}
