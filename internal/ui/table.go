package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/tadamo/podres/internal/kube"
	"github.com/tadamo/podres/internal/threshold"
	"k8s.io/apimachinery/pkg/api/resource"
)

// column widths in terminal characters
const (
	colPod       = 38
	colContainer = 16
	colVal       = 9 // CPU-REQ, CPU-LIM, CPU-USE, MEM-REQ, MEM-LIM, MEM-USE
	colPct       = 7 // CPU%, MEM%
)

var columnHeaders = []string{
	"POD", "CONTAINER",
	"CPU-REQ", "CPU-LIM", "CPU-USE", "CPU%",
	"MEM-REQ", "MEM-LIM", "MEM-USE", "MEM%",
}

var columnWidths = []int{
	colPod, colContainer,
	colVal, colVal, colVal, colPct,
	colVal, colVal, colVal, colPct,
}

type warningEntry struct {
	pod, container, msg string
}

// Render builds the full terminal output from pod specs, live metrics, and display config.
// metrics may be nil when metrics-server is unavailable; usage columns will show "N/A".
func Render(
	namespace string,
	pods []kube.PodSpec,
	metrics map[string]kube.PodMetrics,
	thresh threshold.Config,
	st Styles,
) string {
	var sb strings.Builder

	sb.WriteString(renderStatusLine(namespace, st))
	sb.WriteString("\n\n")
	sb.WriteString(renderHeaderRow(st))
	sb.WriteString("\n")

	var warns []warningEntry
	for _, pod := range pods {
		var pm *kube.PodMetrics
		if metrics != nil {
			if m, ok := metrics[pod.Name]; ok {
				pm = &m
			}
		}
		rows, w := renderPodRows(pod, pm, thresh, st)
		sb.WriteString(rows)
		warns = append(warns, w...)
	}

	if len(warns) > 0 {
		sb.WriteString("\n")
		sb.WriteString(st.Warn.Render("⚠  Warnings:"))
		sb.WriteString("\n")
		for _, w := range warns {
			fmt.Fprintf(&sb, "   %s (%s): %s\n",
				st.Warn.Render(w.container), w.pod, w.msg)
		}
	}

	return sb.String()
}

func renderStatusLine(namespace string, st Styles) string {
	now := time.Now()
	tz, _ := now.Zone()
	return st.StatusLine.Render(fmt.Sprintf(
		"NAMESPACE: %-30s Refreshed: %s   TZ: %s",
		namespace, now.Format("15:04:05"), tz,
	))
}

func renderHeaderRow(st Styles) string {
	cells := make([]string, len(columnHeaders))
	for i, h := range columnHeaders {
		cells[i] = st.Header.Width(columnWidths[i]).Render(h)
	}
	return strings.Join(cells, " ")
}

func renderPodRows(
	pod kube.PodSpec,
	pm *kube.PodMetrics,
	thresh threshold.Config,
	st Styles,
) (string, []warningEntry) {
	var sb strings.Builder
	var warns []warningEntry

	podStyle := st.PodName
	if pod.Restarts > 0 {
		podStyle = st.PodRestart
	}

	for i, c := range pod.Containers {
		// Only the first container row shows the pod name.
		podLabel := ""
		if i == 0 {
			podLabel = truncate(pod.Name, colPod)
		}

		var cm *kube.ContainerMetrics
		if pm != nil {
			for j := range pm.Containers {
				if pm.Containers[j].Name == c.Name {
					cm = &pm.Containers[j]
					break
				}
			}
		}

		metricsAvail := cm != nil
		cpuUseStr, cpuPctStr, cpuLvl := usageCells(
			metricsAvail,
			maybeMilliValue(cm, true),
			c.CPULimit.MilliValue(),
			c.CPURequest.MilliValue(),
			thresh,
			fmtMilliCPU,
		)
		memUseStr, memPctStr, memLvl := usageCells(
			metricsAvail,
			maybeMilliValue(cm, false),
			c.MemLimit.Value(),
			c.MemRequest.Value(),
			thresh,
			fmtBytes,
		)

		if cpuLvl >= threshold.LevelWarn {
			warns = append(warns, warningEntry{pod.Name, c.Name,
				fmt.Sprintf("CPU %s — %s", cpuPctStr, levelLabel(cpuLvl))})
		}
		if memLvl >= threshold.LevelWarn {
			warns = append(warns, warningEntry{pod.Name, c.Name,
				fmt.Sprintf("MEM %s — %s", memPctStr, levelLabel(memLvl))})
		}

		cStyle := st.Container
		if isSidecar(c.Name) {
			cStyle = st.Sidecar
		}
		pStyle := st.PlainCell
		if i == 0 {
			pStyle = podStyle
		}

		cells := []string{
			pStyle.Width(colPod).Render(podLabel),
			cStyle.Width(colContainer).Render(truncate(c.Name, colContainer)),
			st.PlainCell.Width(colVal).Render(quantityStr(c.CPURequest)),
			st.PlainCell.Width(colVal).Render(quantityStr(c.CPULimit)),
			st.PlainCell.Width(colVal).Render(cpuUseStr),
			levelStyle(st, cpuLvl).Width(colPct).Render(cpuPctStr),
			st.PlainCell.Width(colVal).Render(quantityStr(c.MemRequest)),
			st.PlainCell.Width(colVal).Render(quantityStr(c.MemLimit)),
			st.PlainCell.Width(colVal).Render(memUseStr),
			levelStyle(st, memLvl).Width(colPct).Render(memPctStr),
		}
		sb.WriteString(strings.Join(cells, " "))
		sb.WriteString("\n")
	}

	return sb.String(), warns
}

// usageCells returns the formatted usage string, percentage string, and threshold level.
func usageCells(
	avail bool,
	usage, limit, request int64,
	thresh threshold.Config,
	fmtFn func(int64) string,
) (useStr, pctStr string, lvl threshold.Level) {
	if !avail {
		return "N/A", "N/A", threshold.LevelOK
	}
	useStr = fmtFn(usage)

	divisor := limit
	if divisor == 0 {
		divisor = request
	}
	if divisor == 0 {
		return useStr, "N/A", threshold.LevelOK
	}

	pct := float64(usage) / float64(divisor) * 100
	return useStr, fmt.Sprintf("%.0f%%", pct), thresh.Classify(pct)
}

// maybeMilliValue returns the CPU (milli) or memory (raw) value from ContainerMetrics,
// or 0 if cm is nil.
func maybeMilliValue(cm *kube.ContainerMetrics, cpu bool) int64 {
	if cm == nil {
		return 0
	}
	if cpu {
		return cm.CPUUsage.MilliValue()
	}
	return cm.MemUsage.Value()
}

func levelStyle(st Styles, lvl threshold.Level) lipgloss.Style {
	switch lvl {
	case threshold.LevelWarn:
		return st.Warn
	case threshold.LevelCrit:
		return st.Crit
	default:
		return st.OK
	}
}

func levelLabel(lvl threshold.Level) string {
	if lvl >= threshold.LevelCrit {
		return "exceeding threshold"
	}
	return "approaching threshold"
}

func isSidecar(name string) bool {
	return strings.Contains(name, "istio-proxy") ||
		strings.Contains(name, "envoy") ||
		strings.Contains(name, "linkerd-proxy")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-2] + ".."
}

// quantityStr formats a resource.Quantity for display, returning "—" for zero values.
func quantityStr(q resource.Quantity) string {
	if q.IsZero() {
		return "—"
	}
	return q.String()
}

func fmtMilliCPU(m int64) string {
	return fmt.Sprintf("%dm", m)
}

func fmtBytes(b int64) string {
	const (
		Mi = 1024 * 1024
		Gi = 1024 * 1024 * 1024
	)
	switch {
	case b >= Gi:
		return fmt.Sprintf("%.1fGi", float64(b)/Gi)
	case b >= Mi:
		return fmt.Sprintf("%dMi", b/Mi)
	default:
		return fmt.Sprintf("%dB", b)
	}
}
