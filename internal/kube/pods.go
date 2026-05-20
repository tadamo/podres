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
	Name              string
	Status            string // "Running", "Waiting", "Terminated", "Unknown"
	Ready             bool
	Restarts          int32
	LastRestartReason string // e.g. "OOMKilled", "Error" — empty when restarts == 0
	CPURequest        resource.Quantity
	CPULimit          resource.Quantity
	MemRequest        resource.Quantity
	MemLimit          resource.Quantity
}

// PodSpec holds a pod and its containers' resource specs.
type PodSpec struct {
	Namespace  string
	Name       string
	Phase      string // "Pending", "Running", "Succeeded", "Failed", "Unknown"
	Restarts   int32
	Containers []ContainerSpec
}

// ListPods returns resource specs for all pods in the given namespace, optionally filtered by a label selector.
func (c *Client) ListPods(namespace, selector string) ([]PodSpec, error) {
	podList, err := c.kube.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: selector,
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

func containerStateStr(s corev1.ContainerState) string {
	switch {
	case s.Running != nil:
		return "Running"
	case s.Waiting != nil:
		return "Waiting"
	case s.Terminated != nil:
		return "Terminated"
	default:
		return "Unknown"
	}
}

func podToSpec(pod corev1.Pod) PodSpec {
	var restarts int32
	type containerInfo struct {
		state             string
		ready             bool
		restarts          int32
		lastRestartReason string
	}

	lastReason := func(cs corev1.ContainerStatus) string {
		if cs.RestartCount > 0 && cs.LastTerminationState.Terminated != nil {
			return cs.LastTerminationState.Terminated.Reason
		}
		return ""
	}

	infoMap := make(map[string]containerInfo)
	for _, cs := range pod.Status.ContainerStatuses {
		restarts += cs.RestartCount
		infoMap[cs.Name] = containerInfo{containerStateStr(cs.State), cs.Ready, cs.RestartCount, lastReason(cs)}
	}
	// Native sidecar init containers (restartPolicy: Always, k8s 1.29+) have their
	// own status slice; include them so sidecars like istio-proxy are not invisible.
	for _, cs := range pod.Status.InitContainerStatuses {
		if _, already := infoMap[cs.Name]; already {
			continue
		}
		restarts += cs.RestartCount
		infoMap[cs.Name] = containerInfo{containerStateStr(cs.State), cs.Ready, cs.RestartCount, lastReason(cs)}
	}

	appendContainer := func(containers []ContainerSpec, c corev1.Container) []ContainerSpec {
		info := infoMap[c.Name]
		if info.state == "" {
			info.state = "Unknown"
		}
		return append(containers, ContainerSpec{
			Name:              c.Name,
			Status:            info.state,
			Ready:             info.ready,
			Restarts:          info.restarts,
			LastRestartReason: info.lastRestartReason,
			CPURequest:        quantityOrZero(c.Resources.Requests, corev1.ResourceCPU),
			CPULimit:          quantityOrZero(c.Resources.Limits, corev1.ResourceCPU),
			MemRequest:        quantityOrZero(c.Resources.Requests, corev1.ResourceMemory),
			MemLimit:          quantityOrZero(c.Resources.Limits, corev1.ResourceMemory),
		})
	}

	// Native sidecar init containers run alongside regular containers; show them first.
	containers := make([]ContainerSpec, 0, len(pod.Spec.Containers)+len(pod.Spec.InitContainers))
	for _, c := range pod.Spec.InitContainers {
		if c.RestartPolicy == nil || *c.RestartPolicy != corev1.ContainerRestartPolicyAlways {
			continue
		}
		containers = appendContainer(containers, c)
	}
	for _, c := range pod.Spec.Containers {
		containers = appendContainer(containers, c)
	}

	phase := string(pod.Status.Phase)
	if pod.DeletionTimestamp != nil {
		phase = "Terminating"
	}

	return PodSpec{
		Namespace:  pod.Namespace,
		Name:       pod.Name,
		Phase:      phase,
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
