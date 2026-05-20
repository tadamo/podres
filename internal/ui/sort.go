package ui

import (
	"slices"

	"github.com/tadamo/podres/internal/kube"
)

// SortKey identifies which column drives the pod sort order.
type SortKey int

const (
	SortNone     SortKey = iota
	SortCPU              // aggregate CPU usage %
	SortMem              // aggregate memory usage %
	SortRestarts         // total restart count
	SortName             // pod name, alphabetical
)

func (k SortKey) String() string {
	switch k {
	case SortCPU:
		return "CPU%"
	case SortMem:
		return "MEM%"
	case SortRestarts:
		return "Restarts"
	case SortName:
		return "Name"
	default:
		return ""
	}
}

// ParseSortKey converts a CLI string ("cpu", "mem", "restarts", "name") to a SortKey.
func ParseSortKey(s string) SortKey {
	switch s {
	case "cpu":
		return SortCPU
	case "mem":
		return SortMem
	case "restarts":
		return SortRestarts
	case "name":
		return SortName
	default:
		return SortNone
	}
}

// sortPods returns a sorted copy of pods without modifying the original slice.
// Containers within each pod are also reordered by the same key so the
// "winning" container is always the first row shown for that pod.
func sortPods(pods []kube.PodSpec, metrics map[string]kube.PodMetrics, key SortKey, desc bool) []kube.PodSpec {
	if key == SortNone || len(pods) == 0 {
		return pods
	}
	out := make([]kube.PodSpec, len(pods))
	copy(out, pods)
	slices.SortStableFunc(out, func(a, b kube.PodSpec) int {
		cmp := podCmp(a, b, metrics, key)
		if desc {
			return -cmp
		}
		return cmp
	})
	// Reorder containers within each pod by the same key (skip SortName — pod
	// name sort has no meaningful container ordering).
	if key != SortName {
		for i := range out {
			out[i] = sortPodContainers(out[i], metrics, key, desc)
		}
	}
	return out
}

func sortPodContainers(pod kube.PodSpec, metrics map[string]kube.PodMetrics, key SortKey, desc bool) kube.PodSpec {
	if len(pod.Containers) <= 1 {
		return pod
	}
	var pm *kube.PodMetrics
	if metrics != nil {
		if m, ok := metrics[pod.Name]; ok {
			pm = &m
		}
	}
	containers := make([]kube.ContainerSpec, len(pod.Containers))
	copy(containers, pod.Containers)
	slices.SortStableFunc(containers, func(a, b kube.ContainerSpec) int {
		cmp := containerCmp(a, b, pm, key)
		if desc {
			return -cmp
		}
		return cmp
	})
	pod.Containers = containers
	return pod
}

func containerCmp(a, b kube.ContainerSpec, pm *kube.PodMetrics, key SortKey) int {
	av := containerSortVal(a, pm, key)
	bv := containerSortVal(b, pm, key)
	if av < bv {
		return -1
	}
	if av > bv {
		return 1
	}
	return 0
}

func containerSortVal(c kube.ContainerSpec, pm *kube.PodMetrics, key SortKey) float64 {
	if key == SortRestarts {
		return float64(c.Restarts)
	}
	if pm == nil {
		return -1
	}
	cpu := key == SortCPU
	var lim int64
	if cpu {
		lim = c.CPULimit.MilliValue()
		if lim == 0 {
			lim = c.CPURequest.MilliValue()
		}
	} else {
		lim = c.MemLimit.Value()
		if lim == 0 {
			lim = c.MemRequest.Value()
		}
	}
	if lim == 0 {
		return -1
	}
	var usage int64
	for j := range pm.Containers {
		if pm.Containers[j].Name == c.Name {
			if cpu {
				usage = pm.Containers[j].CPUUsage.MilliValue()
			} else {
				usage = pm.Containers[j].MemUsage.Value()
			}
			break
		}
	}
	return float64(usage) / float64(lim) * 100
}

func podCmp(a, b kube.PodSpec, metrics map[string]kube.PodMetrics, key SortKey) int {
	if key == SortName {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	}
	av := podSortVal(a, metrics, key)
	bv := podSortVal(b, metrics, key)
	if av < bv {
		return -1
	}
	if av > bv {
		return 1
	}
	return 0
}

func podSortVal(pod kube.PodSpec, metrics map[string]kube.PodMetrics, key SortKey) float64 {
	if key == SortRestarts {
		return float64(pod.Restarts)
	}
	var pm *kube.PodMetrics
	if metrics != nil {
		if m, ok := metrics[pod.Name]; ok {
			pm = &m
		}
	}
	if pm == nil {
		return -1 // no metrics: sort to bottom
	}
	cpu := key == SortCPU
	// Sort pods by their highest individual container percentage so the value
	// corresponds to something visible in the CPU%/MEM% column.
	maxPct := -1.0
	for _, c := range pod.Containers {
		var lim int64
		if cpu {
			lim = c.CPULimit.MilliValue()
			if lim == 0 {
				lim = c.CPURequest.MilliValue()
			}
		} else {
			lim = c.MemLimit.Value()
			if lim == 0 {
				lim = c.MemRequest.Value()
			}
		}
		if lim == 0 {
			continue
		}
		var usage int64
		for j := range pm.Containers {
			if pm.Containers[j].Name == c.Name {
				if cpu {
					usage = pm.Containers[j].CPUUsage.MilliValue()
				} else {
					usage = pm.Containers[j].MemUsage.Value()
				}
				break
			}
		}
		if pct := float64(usage) / float64(lim) * 100; pct > maxPct {
			maxPct = pct
		}
	}
	return maxPct
}
