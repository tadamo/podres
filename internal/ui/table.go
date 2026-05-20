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
	colPhase     = 14 // "● Running" / "◌ Pending" / "✔ Succeeded" / "✕ Failed"
	colDivider   = 1  // thin visual separator
	colStatus    = 13 // "● Running" / "◌ Waiting" / "✕ Terminated"
	colReady     = 6  // "✔" / "✘"
	colRestarts  = 9  // restart count
	colVal       = 9  // CPU-REQ, CPU-LIM, CPU-USE, MEM-REQ, MEM-LIM, MEM-USE
	colPct       = 7  // CPU%, MEM%
)

// fixedWidths holds the widths of all columns except POD and CONTAINER,
// in the order they appear after those two variable-width columns.
// Index mapping: 0=POD(variable), 1=PHASE, 2=DIVIDER, 3=CONTAINER(variable),
// 4=STATUS, 5=READY, 6=RESTARTS, 7..10=CPU cols, 11..14=MEM cols.
var columnHeaders = []string{
	"POD", "PHASE", "│", "CONTAINER", "STATE", "READY", "RESTARTS",
	"CPU-REQ", "CPU-LIM", "CPU-USE", "CPU%",
	"MEM-REQ", "MEM-LIM", "MEM-USE", "MEM%",
}

// layout holds the effective widths for the two variable-width columns.
type layout struct {
	podCol       int
	containerCol int
}

// newLayout returns a layout using the default column widths, or in wide mode
// expands them to fit the longest pod and container names in the data.
func newLayout(pods []kube.PodSpec, wide bool) layout {
	if !wide {
		return layout{colPod, colContainer}
	}
	pc, cc := colPod, colContainer
	for _, pod := range pods {
		if n := len(pod.Name); n > pc {
			pc = n
		}
		for _, c := range pod.Containers {
			if n := len(c.Name); n > cc {
				cc = n
			}
		}
	}
	return layout{pc, cc}
}

// colWidths returns the full ordered slice of column widths for the given layout.
func (l layout) colWidths() []int {
	return []int{
		l.podCol, colPhase, colDivider, l.containerCol, colStatus, colReady, colRestarts,
		colVal, colVal, colVal, colPct,
		colVal, colVal, colVal, colPct,
	}
}

// totalWidth returns the total rendered width including inter-cell spaces.
func (l layout) totalWidth() int {
	widths := l.colWidths()
	w := len(widths) - 1 // spaces between cells
	for _, cw := range widths {
		w += cw
	}
	return w
}

type warningEntry struct {
	pod, container, msg string
}

// Render builds the full terminal output from pod specs, live metrics, and display config.
// metrics may be nil when metrics-server is unavailable; usage columns will show "N/A".
// quota may be nil when no ResourceQuota exists for the namespace.
// selector, when non-empty, suppresses the ResourceQuota row because the totals only
// cover filtered pods and the percentage would be misleading.
func Render(
	namespace, cluster, user, selector string,
	pods []kube.PodSpec,
	metrics map[string]kube.PodMetrics,
	quota *kube.NamespaceQuota,
	thresh threshold.Config,
	st Styles,
	podDividers bool,
	wide bool,
) string {
	var sb strings.Builder

	lay := newLayout(pods, wide)

	sb.WriteString(renderStatusLine(namespace, cluster, user, st, lay))
	sb.WriteString("\n\n")
	sb.WriteString(renderHeaderRow(st, lay))
	sb.WriteString("\n")

	totals := computeTotals(pods, metrics)

	var warns []warningEntry
	for _, pod := range pods {
		if podDividers {
			sb.WriteString(renderPodDivider(st, lay))
		}
		var pm *kube.PodMetrics
		if metrics != nil {
			if m, ok := metrics[pod.Name]; ok {
				pm = &m
			}
		}
		rows, w := renderPodRows(pod, pm, thresh, st, lay)
		sb.WriteString(rows)
		warns = append(warns, w...)
	}

	sb.WriteString(renderThickDivider(st, lay))
	sb.WriteString(renderTotalsRow(totals, st, lay))
	sb.WriteString("\n")
	if selector != "" {
		sb.WriteString(st.Divider.Render("◌  ResourceQuota hidden (label selector active)"))
		sb.WriteString("\n")
	} else if quota != nil {
		sb.WriteString(renderPodDivider(st, lay))
		sb.WriteString(renderQuotaRow(quota, totals, thresh, st, lay))
		sb.WriteString("\n")
	} else {
		sb.WriteString(st.Divider.Render("◌  No ResourceQuota set for this namespace"))
		sb.WriteString("\n")
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

func renderStatusLine(namespace, cluster, user string, st Styles, lay layout) string {
	now := time.Now()
	tz, _ := now.Zone()

	left := st.StatusLine.Render(fmt.Sprintf(
		"⎈  NAMESPACE: %s     ⬡  CLUSTER: %s     ◉  USER: %s",
		namespace, cluster, user,
	))
	right := st.Dim.Render(fmt.Sprintf("Refreshed: %s   TZ: %s",
		now.Format("01/02/2006 3:04:05 PM"), tz,
	))

	gap := max(1, lay.totalWidth()-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
}

func renderHeaderRow(st Styles, lay layout) string {
	widths := lay.colWidths()
	cells := make([]string, len(columnHeaders))
	for i, h := range columnHeaders {
		style := st.Header
		if h == "│" {
			style = st.Divider
		}
		cells[i] = style.Width(widths[i]).Render(h)
	}
	return strings.Join(cells, " ")
}

type tableTotals struct {
	cpuReqMilli int64
	cpuLimMilli int64
	cpuUseMilli int64
	cpuUseAvail bool
	memReqBytes int64
	memLimBytes int64
	memUseBytes int64
	memUseAvail bool
}

func computeTotals(pods []kube.PodSpec, metrics map[string]kube.PodMetrics) tableTotals {
	var t tableTotals
	t.cpuUseAvail = metrics != nil
	t.memUseAvail = metrics != nil
	for _, pod := range pods {
		var pm *kube.PodMetrics
		if metrics != nil {
			if m, ok := metrics[pod.Name]; ok {
				pm = &m
			}
		}
		for _, c := range pod.Containers {
			t.cpuReqMilli += c.CPURequest.MilliValue()
			t.cpuLimMilli += c.CPULimit.MilliValue()
			t.memReqBytes += c.MemRequest.Value()
			t.memLimBytes += c.MemLimit.Value()
			if pm != nil {
				for j := range pm.Containers {
					if pm.Containers[j].Name == c.Name {
						t.cpuUseMilli += pm.Containers[j].CPUUsage.MilliValue()
						t.memUseBytes += pm.Containers[j].MemUsage.Value()
						break
					}
				}
			}
		}
	}
	return t
}

func renderThickDivider(st Styles, lay layout) string {
	return st.Header.Render(strings.Repeat("═", lay.totalWidth())) + "\n"
}

func renderTotalsRow(t tableTotals, st Styles, lay layout) string {
	fmtOrDash := func(v int64, fn func(int64) string) string {
		if v == 0 {
			return "—"
		}
		return fn(v)
	}

	cpuUseStr := "N/A"
	if t.cpuUseAvail {
		cpuUseStr = fmtOrDash(t.cpuUseMilli, fmtMilliCPU)
	}
	memUseStr := "N/A"
	if t.memUseAvail {
		memUseStr = fmtOrDash(t.memUseBytes, fmtBytes)
	}

	cells := []string{
		st.Header.Width(lay.podCol).Render("TOTAL"),
		st.PlainCell.Width(colPhase).Render(""),
		st.Divider.Width(colDivider).Render("│"),
		st.PlainCell.Width(lay.containerCol).Render(""),
		st.PlainCell.Width(colStatus).Render(""),
		st.PlainCell.Width(colReady).Render(""),
		st.PlainCell.Width(colRestarts).Render(""),
		st.Header.Width(colVal).Render(fmtOrDash(t.cpuReqMilli, fmtMilliCPU)),
		st.Header.Width(colVal).Render(fmtOrDash(t.cpuLimMilli, fmtMilliCPU)),
		st.Header.Width(colVal).Render(cpuUseStr),
		st.PlainCell.Width(colPct).Render("—"),
		st.Header.Width(colVal).Render(fmtOrDash(t.memReqBytes, fmtBytes)),
		st.Header.Width(colVal).Render(fmtOrDash(t.memLimBytes, fmtBytes)),
		st.Header.Width(colVal).Render(memUseStr),
		st.PlainCell.Width(colPct).Render("—"),
	}
	return strings.Join(cells, " ")
}

func renderQuotaRow(q *kube.NamespaceQuota, t tableTotals, thresh threshold.Config, st Styles, lay layout) string {
	qStr := func(v resource.Quantity) string {
		if v.IsZero() {
			return "—"
		}
		return v.String()
	}
	pctCell := func(used, quota int64) (string, threshold.Level) {
		if quota == 0 {
			return "—", threshold.LevelOK
		}
		pct := float64(used) / float64(quota) * 100
		return fmt.Sprintf("%.0f%%", pct), thresh.Classify(pct)
	}

	cpuPct, cpuLvl := pctCell(t.cpuReqMilli, q.CPURequest.MilliValue())
	memPct, memLvl := pctCell(t.memReqBytes, q.MemRequest.Value())

	cells := []string{
		st.Header.Width(lay.podCol).Render("ResourceQuota"),
		st.PlainCell.Width(colPhase).Render(""),
		st.Divider.Width(colDivider).Render("│"),
		st.PlainCell.Width(lay.containerCol).Render(""),
		st.PlainCell.Width(colStatus).Render(""),
		st.PlainCell.Width(colReady).Render(""),
		st.PlainCell.Width(colRestarts).Render(""),
		st.Header.Width(colVal).Render(qStr(q.CPURequest)),
		st.Header.Width(colVal).Render(qStr(q.CPULimit)),
		st.PlainCell.Width(colVal).Render(""),
		levelStyle(st, cpuLvl).Width(colPct).Render(cpuPct),
		st.Header.Width(colVal).Render(qStr(q.MemRequest)),
		st.Header.Width(colVal).Render(qStr(q.MemLimit)),
		st.PlainCell.Width(colVal).Render(""),
		levelStyle(st, memLvl).Width(colPct).Render(memPct),
	}
	return strings.Join(cells, " ")
}

func renderPodDivider(st Styles, lay layout) string {
	return st.Divider.Render(strings.Repeat("─", lay.totalWidth())) + "\n"
}

func renderPodRows(
	pod kube.PodSpec,
	pm *kube.PodMetrics,
	thresh threshold.Config,
	st Styles,
	lay layout,
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
			podLabel = truncate(pod.Name, lay.podCol)
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

		phaseLabel := ""
		var phaseSym string
		var phaseStyle lipgloss.Style
		if i == 0 {
			phaseSym, phaseStyle = podPhaseCell(pod.Phase, st)
			phaseLabel = phaseSym
		} else {
			phaseStyle = st.PlainCell
		}

		statusSym, statusStyle := containerStatusCell(c.Status, pod.Phase, st)
		readySym, readyStyle := containerReadyCell(c.Ready, pod.Phase, st)
		restartsStr, restartsStyle := containerRestartsCell(c.Restarts, st)
		cells := []string{
			pStyle.Width(lay.podCol).Render(podLabel),
			phaseStyle.Width(colPhase).Render(phaseLabel),
			st.Divider.Width(colDivider).Render("│"),
			cStyle.Width(lay.containerCol).Render(truncate(c.Name, lay.containerCol)),
			statusStyle.Width(colStatus).Render(statusSym),
			readyStyle.Width(colReady).Render(readySym),
			restartsStyle.Width(colRestarts).Render(restartsStr),
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

func podPhaseCell(phase string, st Styles) (string, lipgloss.Style) {
	switch phase {
	case "Running":
		return "● Running", st.OK
	case "Pending":
		return "◌ Pending", st.Warn
	case "Succeeded":
		return "✔ Succeeded", st.OK
	case "Failed":
		return "✕ Failed", st.Crit
	case "Terminating":
		return "◌ Terminating", st.Warn
	default:
		return "? Unknown", st.PlainCell
	}
}

func containerReadyCell(ready bool, podPhase string, st Styles) (string, lipgloss.Style) {
	if ready {
		return "✔", st.OK
	}
	if podPhase == "Succeeded" {
		return "—", st.Dim
	}
	if podPhase == "Terminating" {
		return "✘", st.PlainCell
	}
	return "✘", st.Crit
}

func containerRestartsCell(restarts int32, st Styles) (string, lipgloss.Style) {
	s := fmt.Sprintf("%d", restarts)
	if restarts > 0 {
		return s, st.Warn
	}
	return s, st.PlainCell
}

func containerStatusCell(status, podPhase string, st Styles) (string, lipgloss.Style) {
	switch status {
	case "Running":
		return "● Running", st.OK
	case "Waiting":
		return "◌ Waiting", st.Warn
	case "Terminated":
		if podPhase == "Succeeded" || podPhase == "Terminating" {
			return "✔ Terminated", st.OK
		}
		return "✕ Terminated", st.Crit
	default:
		return "? Unknown", st.PlainCell
	}
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
