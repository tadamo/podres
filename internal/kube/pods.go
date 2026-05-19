package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ContainerSpec holds a container's resource requests and limits.
type ContainerSpec struct {
	Name       string
	Status     string // "Running", "Waiting", "Terminated", "Unknown"
	Ready      bool
	CPURequest resource.Quantity
	CPULimit   resource.Quantity
	MemRequest resource.Quantity
	MemLimit   resource.Quantity
}

// PodSpec holds a pod and its containers' resource specs.
type PodSpec struct {
	Namespace  string
	Name       string
	Phase      string // "Pending", "Running", "Succeeded", "Failed", "Unknown"
	Restarts   int32
	Containers []ContainerSpec
}

// ListPods returns resource specs for all running pods in the given namespace.
func (c *Client) ListPods(namespace string) ([]PodSpec, error) {
	podList, err := c.kube.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		FieldSelector: "status.phase=Running",
	})
	if err != nil {
		return nil, fmt.Errorf("list pods in %q: %w", namespace, err)
	}

	specs := make([]PodSpec, 0, len(podList.Items))
	for _, pod := range podList.Items {
		specs = append(specs, podToSpec(pod))
	}
	return specs, nil
}

func podToSpec(pod corev1.Pod) PodSpec {
	var restarts int32
	type containerInfo struct {
		state string
		ready bool
	}
	infoMap := make(map[string]containerInfo, len(pod.Status.ContainerStatuses))
	for _, cs := range pod.Status.ContainerStatuses {
		restarts += cs.RestartCount
		var state string
		switch {
		case cs.State.Running != nil:
			state = "Running"
		case cs.State.Waiting != nil:
			state = "Waiting"
		case cs.State.Terminated != nil:
			state = "Terminated"
		default:
			state = "Unknown"
		}
		infoMap[cs.Name] = containerInfo{state: state, ready: cs.Ready}
	}

	containers := make([]ContainerSpec, 0, len(pod.Spec.Containers))
	for _, c := range pod.Spec.Containers {
		info := infoMap[c.Name]
		if info.state == "" {
			info.state = "Unknown"
		}
		containers = append(containers, ContainerSpec{
			Name:   c.Name,
			Status: info.state,
			Ready:  info.ready,
			CPURequest: quantityOrZero(c.Resources.Requests, corev1.ResourceCPU),
			CPULimit:   quantityOrZero(c.Resources.Limits, corev1.ResourceCPU),
			MemRequest: quantityOrZero(c.Resources.Requests, corev1.ResourceMemory),
			MemLimit:   quantityOrZero(c.Resources.Limits, corev1.ResourceMemory),
		})
	}

	return PodSpec{
		Namespace:  pod.Namespace,
		Name:       pod.Name,
		Phase:      string(pod.Status.Phase),
		Restarts:   restarts,
		Containers: containers,
	}
}

func quantityOrZero(resources corev1.ResourceList, name corev1.ResourceName) resource.Quantity {
	if q, ok := resources[name]; ok {
		return q
	}
	return resource.Quantity{}
}
