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
	colNamespace = 20 // NAMESPACE column (all-namespaces mode only)
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

var columnHeaders = []string{
	"POD", "PHASE", "│", "CONTAINER", "STATE", "READY", "RESTARTS",
	"CPU-REQ", "CPU-LIM", "CPU-USE", "CPU%",
	"MEM-REQ", "MEM-LIM", "MEM-USE", "MEM%",
}

func effectiveHeaders(allNamespaces bool) []string {
	if !allNamespaces {
		return columnHeaders
	}
	h := make([]string, 0, len(columnHeaders)+1)
	h = append(h, "NAMESPACE")
	h = append(h, columnHeaders...)
	return h
}

// layout holds the effective widths for the variable-width columns.
type layout struct {
	podCol       int
	containerCol int
	namespaceCol int // > 0 only when allNamespaces
	allNamespaces bool
}

// newLayout returns a layout using the default column widths, or in wide mode
// expands them to fit the longest pod, container, and namespace names in the data.
func newLayout(pods []kube.PodSpec, wide, allNamespaces bool) layout {
	l := layout{
		podCol:        colPod,
		containerCol:  colContainer,
		allNamespaces: allNamespaces,
	}
	if allNamespaces {
		l.namespaceCol = colNamespace
	}
	if !wide {
		return l
	}
	for _, pod := range pods {
		if n := len(pod.Name); n > l.podCol {
			l.podCol = n
		}
		if allNamespaces {
			if n := len(pod.Namespace); n > l.namespaceCol {
				l.namespaceCol = n
			}
		}
		for _, c := range pod.Containers {
			if n := len(c.Name); n > l.containerCol {
				l.containerCol = n
			}
		}
	}
	return l
}

// colWidths returns the full ordered slice of column widths for the given layout.
func (l layout) colWidths() []int {
	base := []int{
		l.podCol, colPhase, colDivider, l.containerCol, colStatus, colReady, colRestarts,
		colVal, colVal, colVal, colPct,
		colVal, colVal, colVal, colPct,
	}
	if l.allNamespaces {
		return append([]int{l.namespaceCol}, base...)
	}
	return base
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

// RenderFixedHeader returns the pinned top portion: status line, ResourceQuota
// section (or a placeholder message), sort hint, column headers, and divider.
// Its line count is variable — use strings.Count(result, "\n") to measure it.
func RenderFixedHeader(
	namespace, cluster, user, selector string,
	pods []kube.PodSpec,
	metrics map[string]kube.PodMetrics,
	quota *kube.NamespaceQuota,
	thresh threshold.Config,
	st Styles,
	wide bool,
	allNamespaces bool,
	sortKey SortKey,
	sortDesc bool,
) string {
	lay := newLayout(pods, wide, allNamespaces)
	var sb strings.Builder
	displayNS := namespace
	if allNamespaces {
		displayNS = "All Namespaces"
	}
	sb.WriteString(renderStatusLine(displayNS, cluster, user, st, lay))
	sb.WriteString("\n\n")
	switch {
	case allNamespaces:
		sb.WriteString(st.Divider.Render("◌  ResourceQuota not shown in all-namespaces view"))
		sb.WriteString("\n")
	case selector != "":
		sb.WriteString(st.Divider.Render("◌  ResourceQuota hidden (label selector active)"))
		sb.WriteString("\n")
	case quota != nil:
		sb.WriteString(renderQuotaSection(quota, computeTotals(pods, metrics), thresh, st))
	default:
		sb.WriteString(st.Divider.Render("◌  No ResourceQuota set for this namespace"))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	sb.WriteString(renderHeaderRow(st, lay, sortKey, sortDesc))
	sb.WriteString("\n")
	sb.WriteString(renderThickDivider(st, lay))
	return sb.String()
}

// RenderBody returns the scrollable pod rows only (no divider or TOTAL line).
func RenderBody(
	pods []kube.PodSpec,
	metrics map[string]kube.PodMetrics,
	thresh threshold.Config,
	st Styles,
	podDividers bool,
	wide bool,
	allNamespaces bool,
) string {
	lay := newLayout(pods, wide, allNamespaces)
	var sb strings.Builder
	for _, pod := range pods {
		if podDividers {
			sb.WriteString(renderPodDivider(st, lay))
		}
		var pm *kube.PodMetrics
		if metrics != nil {
			if m, ok := metrics[pod.Namespace+"/"+pod.Name]; ok {
				pm = &m
			}
		}
		sb.WriteString(renderPodRows(pod, pm, thresh, st, lay))
	}
	return sb.String()
}

// RenderTotalArea returns the thick divider and TOTAL row as two pinned lines.
func RenderTotalArea(pods []kube.PodSpec, metrics map[string]kube.PodMetrics, st Styles, wide bool, allNamespaces bool) string {
	lay := newLayout(pods, wide, allNamespaces)
	totals := computeTotals(pods, metrics)
	return renderThickDivider(st, lay) + renderTotalsRow(totals, st, lay) + "\n"
}

// renderWatchFooterBody returns the sort hint line, right-aligned to min(termWidth, lay.totalWidth()).
func renderWatchFooterBody(pods []kube.PodSpec, st Styles, wide bool, allNamespaces bool, sortKey SortKey, sortDesc bool, termWidth int) string {
	lay := newLayout(pods, wide, allNamespaces)
	return renderSortHint(sortKey, sortDesc, st, min(termWidth, lay.totalWidth()), allNamespaces) + "\n"
}

// RenderFixedFooter returns the pinned bottom portion for no-watch mode.
func RenderFixedFooter(pods []kube.PodSpec, st Styles, wide bool, allNamespaces bool, sortKey SortKey, sortDesc bool) string {
	lay := newLayout(pods, wide, allNamespaces)
	return "\n" + renderWatchFooterBody(pods, st, wide, allNamespaces, sortKey, sortDesc, lay.totalWidth())
}

// Render returns the complete output for --no-watch mode (header + body + total + footer).
func Render(
	namespace, cluster, user, selector string,
	pods []kube.PodSpec,
	metrics map[string]kube.PodMetrics,
	quota *kube.NamespaceQuota,
	thresh threshold.Config,
	st Styles,
	podDividers bool,
	wide bool,
	allNamespaces bool,
	sortKey SortKey,
	sortDesc bool,
) string {
	return RenderFixedHeader(namespace, cluster, user, selector, pods, metrics, quota, thresh, st, wide, allNamespaces, sortKey, sortDesc) +
		RenderBody(pods, metrics, thresh, st, podDividers, wide, allNamespaces) +
		RenderTotalArea(pods, metrics, st, wide, allNamespaces) +
		RenderFixedFooter(pods, st, wide, allNamespaces, sortKey, sortDesc)
}

// renderSortHint returns a right-aligned dim line with the arrow embedded in the active key.
// width is the column to right-align to (caller passes min(termWidth, lay.totalWidth())).
func renderSortHint(key SortKey, desc bool, st Styles, width int, allNamespaces bool) string {
	arrow := "↓"
	if !desc {
		arrow = "↑"
	}
	mark := func(k SortKey, label string) string {
		if key == k {
			return arrow + label
		}
		return label
	}
	keys := fmt.Sprintf("c=%s · m=%s · r=%s · p=%s",
		mark(SortCPU, "cpu"),
		mark(SortMem, "mem"),
		mark(SortRestarts, "restarts"),
		mark(SortName, "name"),
	)
	if allNamespaces {
		keys += fmt.Sprintf(" · n=%s", mark(SortNamespace, "namespace"))
	}
	if key != SortNone {
		keys += " · 0=off"
	}
	line := "Sort by: " + keys
	rendered := st.Dim.Render(line)
	pad := max(0, width-lipgloss.Width(rendered))
	return strings.Repeat(" ", pad) + rendered
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

// sortHeaderCol returns the effectiveHeaders index that corresponds to the given sort key.
func sortHeaderCol(key SortKey, allNamespaces bool) int {
	offset := 0
	if allNamespaces {
		offset = 1
	}
	switch key {
	case SortNamespace:
		return 0 // only present when allNamespaces
	case SortName:
		return 0 + offset // POD
	case SortRestarts:
		return 6 + offset // RESTARTS
	case SortCPU:
		return 10 + offset // CPU%
	case SortMem:
		return 14 + offset // MEM%
	default:
		return -1
	}
}

func renderHeaderRow(st Styles, lay layout, sortKey SortKey, sortDesc bool) string {
	headers := effectiveHeaders(lay.allNamespaces)
	widths := lay.colWidths()
	cells := make([]string, len(headers))
	activeCol := sortHeaderCol(sortKey, lay.allNamespaces)
	arrow := "↓"
	if !sortDesc {
		arrow = "↑"
	}
	for i, h := range headers {
		style := st.Header
		if h == "│" {
			style = st.Divider
		}
		label := h
		if i == activeCol {
			label = h + arrow
		}
		cells[i] = style.Width(widths[i]).Render(label)
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
			if m, ok := metrics[pod.Namespace+"/"+pod.Name]; ok {
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
	return st.Divider.Render(strings.Repeat("─", lay.totalWidth())) + "\n"
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
		cpuUseStr = fmtOrDash(t.cpuUseMilli, fmtMilliCPUAuto)
	}
	memUseStr := "N/A"
	if t.memUseAvail {
		memUseStr = fmtOrDash(t.memUseBytes, fmtBytes)
	}

	cells := []string{}
	if lay.allNamespaces {
		cells = append(cells,
			st.Header.Width(lay.namespaceCol).Render("TOTAL"),
			st.PlainCell.Width(lay.podCol).Render(""),
		)
	} else {
		cells = append(cells, st.Header.Width(lay.podCol).Render("TOTAL"))
	}
	cells = append(cells,
		st.PlainCell.Width(colPhase).Render(""),
		st.Divider.Width(colDivider).Render("│"),
		st.PlainCell.Width(lay.containerCol).Render(""),
		st.PlainCell.Width(colStatus).Render(""),
		st.PlainCell.Width(colReady).Render(""),
		st.PlainCell.Width(colRestarts).Render(""),
		st.Header.Width(colVal).Render(fmtOrDash(t.cpuReqMilli, fmtMilliCPUAuto)),
		st.Header.Width(colVal).Render(fmtOrDash(t.cpuLimMilli, fmtMilliCPUAuto)),
		st.Header.Width(colVal).Render(cpuUseStr),
		st.PlainCell.Width(colPct).Render("—"),
		st.Header.Width(colVal).Render(fmtOrDash(t.memReqBytes, fmtBytes)),
		st.Header.Width(colVal).Render(fmtOrDash(t.memLimBytes, fmtBytes)),
		st.Header.Width(colVal).Render(memUseStr),
		st.PlainCell.Width(colPct).Render("—"),
	)
	return strings.Join(cells, " ")
}

func renderQuotaSection(q *kube.NamespaceQuota, t tableTotals, thresh threshold.Config, st Styles) string {
	const (
		qLabel = 16 // fits "usage / allowed"
		qVal   = 16 // fits "128.5Gi / 256Gi"
	)

	qStr := func(v resource.Quantity) string {
		if v.IsZero() {
			return "—"
		}
		return v.String()
	}
	fmtOrDash := func(v int64, fn func(int64) string) string {
		if v == 0 {
			return "—"
		}
		return fn(v)
	}

	// Render "usage / allowed" where usage is threshold-colored and "/ allowed" is dimmed.
	cell := func(used, quota int64, usageStr, quotaStr string) string {
		var lvl threshold.Level
		if quota > 0 {
			lvl = thresh.Classify(float64(used) / float64(quota) * 100)
		}
		content := levelStyle(st, lvl).Render(usageStr) + st.Dim.Render(" / "+quotaStr)
		pad := max(0, qVal-lipgloss.Width(content))
		return content + strings.Repeat(" ", pad)
	}

	sep := st.Divider.Render("│")
	join := func(cells ...string) string { return strings.Join(cells, " ") }

	header := join(
		st.Header.Width(qLabel).Render("ResourceQuota"),
		sep,
		st.Header.Width(qVal).Render("CPU-REQ"),
		st.Header.Width(qVal).Render("CPU-LIM"),
		st.Header.Width(qVal).Render("MEMORY-REQ"),
		st.Header.Width(qVal).Render("MEMORY-LIM"),
	)
	data := join(
		st.Dim.Width(qLabel).Render("usage / allowed"),
		sep,
		cell(t.cpuReqMilli, q.CPURequest.MilliValue(), fmtOrDash(t.cpuReqMilli, fmtMilliCPUAuto), qStr(q.CPURequest)),
		cell(t.cpuLimMilli, q.CPULimit.MilliValue(), fmtOrDash(t.cpuLimMilli, fmtMilliCPUAuto), qStr(q.CPULimit)),
		cell(t.memReqBytes, q.MemRequest.Value(), fmtOrDash(t.memReqBytes, fmtBytes), qStr(q.MemRequest)),
		cell(t.memLimBytes, q.MemLimit.Value(), fmtOrDash(t.memLimBytes, fmtBytes), qStr(q.MemLimit)),
	)
	return header + "\n" + data + "\n"
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
) string {
	var sb strings.Builder

	rowSt := st
	if pod.Phase == "Succeeded" || pod.Phase == "Failed" {
		rowSt = dimStyles(st)
	}

	podStyle := rowSt.PodName

	for i, c := range pod.Containers {
		// Only the first container row shows the pod name (and namespace in all-namespaces mode).
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

		cStyle := rowSt.Container
		pStyle := rowSt.PlainCell
		if i == 0 {
			pStyle = podStyle
		}

		phaseLabel := ""
		var phaseSym string
		var phaseStyle lipgloss.Style
		if i == 0 {
			phaseSym, phaseStyle = podPhaseCell(pod.Phase, rowSt)
			phaseLabel = phaseSym
		} else {
			phaseStyle = rowSt.PlainCell
		}

		statusSym, statusStyle := containerStatusCell(c.Status, pod.Phase, rowSt)
		readySym, readyStyle := containerReadyCell(c.Ready, pod.Phase, rowSt)
		restartsStr, restartsStyle := containerRestartsCell(c.Restarts, c.LastRestartReason, rowSt)

		cells := []string{}
		if lay.allNamespaces {
			nsLabel := ""
			if i == 0 {
				nsLabel = truncate(pod.Namespace, lay.namespaceCol)
			}
			cells = append(cells, rowSt.PlainCell.Width(lay.namespaceCol).Render(nsLabel))
		}
		cells = append(cells,
			pStyle.Width(lay.podCol).Render(podLabel),
			phaseStyle.Width(colPhase).Render(phaseLabel),
			rowSt.Divider.Width(colDivider).Render("│"),
			cStyle.Width(lay.containerCol).Render(truncate(c.Name, lay.containerCol)),
			statusStyle.Width(colStatus).Render(statusSym),
			readyStyle.Width(colReady).Render(readySym),
			restartsStyle.Width(colRestarts).Render(restartsStr),
			rowSt.PlainCell.Width(colVal).Render(quantityStr(c.CPURequest)),
			rowSt.PlainCell.Width(colVal).Render(quantityStr(c.CPULimit)),
			rowSt.PlainCell.Width(colVal).Render(cpuUseStr),
			levelStyle(rowSt, cpuLvl).Width(colPct).Render(cpuPctStr),
			rowSt.PlainCell.Width(colVal).Render(quantityStr(c.MemRequest)),
			rowSt.PlainCell.Width(colVal).Render(quantityStr(c.MemLimit)),
			rowSt.PlainCell.Width(colVal).Render(memUseStr),
			levelStyle(rowSt, memLvl).Width(colPct).Render(memPctStr),
		)
		sb.WriteString(strings.Join(cells, " "))
		sb.WriteString("\n")
	}

	return sb.String()
}

// usageCells returns the formatted usage string, percentage string, and threshold level.
// The percentage string includes a symbol suffix for non-OK levels so threshold violations
// are visible without relying on color alone: ⚠ for warn, ! for crit.
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
	lvl = thresh.Classify(pct)
	pctStr = fmt.Sprintf("%.0f%%", pct)
	switch lvl {
	case threshold.LevelWarn:
		pctStr += "⚠"
	case threshold.LevelCrit:
		pctStr += "‼"
	}
	return useStr, pctStr, lvl
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

// dimStyles returns a copy of st with Faint(true) applied to every style,
// preserving all color and weight attributes so indicators stay recognizable.
func dimStyles(st Styles) Styles {
	d := func(s lipgloss.Style) lipgloss.Style { return s.Faint(true) }
	return Styles{
		OK:        d(st.OK),
		Warn:      d(st.Warn),
		Crit:      d(st.Crit),
		Header:    d(st.Header),
		PodName:   d(st.PodName),
		Container: d(st.Container),
		PlainCell:  d(st.PlainCell),
		Divider:    d(st.Divider),
		StatusLine: d(st.StatusLine),
		Dim:        d(st.Dim),
	}
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

// fmtMilliCPUAuto shows millicores for values under 1 core, otherwise converts
// to cores with up to one decimal place (e.g. 15500m → "15.5", 2000m → "2").
func fmtMilliCPUAuto(m int64) string {
	if m < 1000 {
		return fmt.Sprintf("%dm", m)
	}
	cores := float64(m) / 1000
	if cores == float64(int64(cores)) {
		return fmt.Sprintf("%.0f", cores)
	}
	return fmt.Sprintf("%.1f", cores)
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

func abbreviateReason(r string) string {
	switch r {
	case "OOMKilled":
		return "OOM"
	case "Error":
		return "Err"
	case "ContainerCannotRun":
		return "!Run"
	case "DeadlineExceeded":
		return "Tmout"
	default:
		if len(r) > 5 {
			return r[:5]
		}
		return r
	}
}

func containerRestartsCell(restarts int32, reason string, st Styles) (string, lipgloss.Style) {
	if restarts == 0 {
		return "0", st.PlainCell
	}
	s := fmt.Sprintf("%d", restarts)
	if reason != "" {
		s = fmt.Sprintf("%d %s", restarts, abbreviateReason(reason))
	}
	if reason == "OOMKilled" {
		return s, st.Crit
	}
	return s, st.Warn
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
