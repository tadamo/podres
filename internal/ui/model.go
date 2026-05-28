package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tadamo/podres/internal/kube"
	"github.com/tadamo/podres/internal/threshold"
)

// Model is the Bubbletea application model for watch-mode display.
type Model struct {
	client        *kube.Client
	namespace     string
	selector      string
	cluster       string
	user          string
	thresh        threshold.Config
	styles        Styles
	interval      time.Duration
	noWatch       bool
	podDividers   bool
	wide          bool
	allNamespaces bool

	// sort state
	sortKey  SortKey
	sortDesc bool

	// current display state
	pods    []kube.PodSpec
	metrics map[string]kube.PodMetrics
	quota   *kube.NamespaceQuota
	err     error

	// viewport for the scrollable table body (watch mode only)
	viewport     viewport.Model
	ready        bool
	termWidth    int
	termHeight   int
	tableWidth   int    // lay.totalWidth() at last rebuild — used for dynamic divider rendering
	headerBase   string // fixed header WITHOUT the trailing thick divider (watch mode only)
	footerSuffix string // totals row + padding blank lines + keyboard hint (watch mode only)

	// namespace picker state
	pickerMode    bool
	pickerLoading bool
	namespaces    []string
	pickerCursor  int
	pickerQuery   string
}

// fetchResult carries the outcome of one refresh cycle.
type fetchResult struct {
	pods    []kube.PodSpec
	metrics map[string]kube.PodMetrics
	quota   *kube.NamespaceQuota
	err     error
}

// tickMsg fires when the refresh interval elapses.
type tickMsg struct{}

// namespacesResult carries the outcome of a namespace list fetch.
type namespacesResult struct {
	namespaces []string
	err        error
}

// New returns an initialized Model ready to run.
func New(
	client *kube.Client,
	namespace, selector, cluster, user string,
	thresh threshold.Config,
	styles Styles,
	interval time.Duration,
	noWatch bool,
	podDividers bool,
	wide bool,
	allNamespaces bool,
	initialSort SortKey,
) Model {
	// CPU/mem/restarts default to descending (highest first); name/namespace default to ascending.
	desc := initialSort == SortCPU || initialSort == SortMem || initialSort == SortRestarts
	return Model{
		client:        client,
		namespace:     namespace,
		selector:      selector,
		cluster:       cluster,
		user:          user,
		thresh:        thresh,
		styles:        styles,
		interval:      interval,
		noWatch:       noWatch,
		podDividers:   podDividers,
		wide:          wide,
		allNamespaces: allNamespaces,
		sortKey:       initialSort,
		sortDesc:      desc,
	}
}

// displayNamespace returns the namespace string for UI display.
func (m Model) displayNamespace() string {
	if m.allNamespaces {
		return "All Namespaces"
	}
	return m.namespace
}

// Init triggers the first data fetch immediately on startup.
func (m Model) Init() tea.Cmd {
	return m.fetchCmd()
}

// Update handles incoming messages and drives state transitions.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height)
			m.ready = true
		}
		if m.pods != nil {
			m = m.rebuildViewport()
		}
		return m, nil

	case tea.KeyMsg:
		if m.pickerMode {
			return m.updatePicker(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "c":
			m = m.cycleSort(SortCPU)
			if m.ready && m.pods != nil {
				m = m.rebuildViewport()
			}
			return m, nil
		case "m":
			m = m.cycleSort(SortMem)
			if m.ready && m.pods != nil {
				m = m.rebuildViewport()
			}
			return m, nil
		case "r":
			m = m.cycleSort(SortRestarts)
			if m.ready && m.pods != nil {
				m = m.rebuildViewport()
			}
			return m, nil
		case "p":
			m = m.cycleSort(SortName)
			if m.ready && m.pods != nil {
				m = m.rebuildViewport()
			}
			return m, nil
		case "n":
			if m.allNamespaces {
				m = m.cycleSort(SortNamespace)
				if m.ready && m.pods != nil {
					m = m.rebuildViewport()
				}
				return m, nil
			}
		case "0":
			m.sortKey = SortNone
			m.sortDesc = false
			if m.ready && m.pods != nil {
				m = m.rebuildViewport()
			}
			return m, nil
		case "f":
			if !m.noWatch {
				m.pickerMode = true
				m.pickerLoading = true
				m.pickerQuery = ""
				m.namespaces = nil
				if m.ready && m.pods != nil {
					m = m.rebuildViewport()
				}
				return m, m.fetchNamespacesCmd()
			}
		}
		if !m.noWatch && m.ready {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

	case fetchResult:
		m.pods = msg.pods
		m.metrics = msg.metrics
		m.quota = msg.quota
		m.err = msg.err

		if m.noWatch {
			return m, tea.Quit
		}
		if m.ready && m.pods != nil {
			m = m.rebuildViewport()
		}
		return m, tea.Tick(m.interval, func(time.Time) tea.Msg {
			return tickMsg{}
		})

	case tickMsg:
		return m, m.fetchCmd()

	case namespacesResult:
		m.pickerLoading = false
		if msg.err != nil {
			m.pickerMode = false
			m.err = msg.err
			return m, nil
		}
		m.namespaces = msg.namespaces
		m.pickerCursor = 0
		display := buildDisplayList(filteredNamespaces(msg.namespaces, m.pickerQuery), m.pickerQuery)
		for i, item := range display {
			if (item == allNamespacesEntry && m.allNamespaces) || item == m.namespace {
				m.pickerCursor = i
				break
			}
		}
		if m.ready && m.pods != nil {
			m = m.rebuildViewport()
		}
		return m, nil
	}

	return m, nil
}

// View renders the current state to a string.
func (m Model) View() string {
	if m.err != nil {
		return m.styles.Crit.Render(fmt.Sprintf("error: %v\n", m.err))
	}
	if m.pods == nil {
		return "Loading…\n"
	}
	if m.noWatch {
		sorted := sortPods(m.pods, m.metrics, m.sortKey, m.sortDesc)
		return Render(m.displayNamespace(), m.cluster, m.user, m.selector, sorted, m.metrics, m.quota, m.thresh, m.styles, m.podDividers, m.wide, m.allNamespaces, m.sortKey, m.sortDesc, m.termWidth)
	}
	if !m.ready {
		return "Loading…\n"
	}
	// The explicit "\n" terminates the viewport's last rendered line.
	// renderTopDivider / renderBotDivider embed live scroll indicators when needed.
	return m.headerBase + m.renderTopDivider() + m.viewport.View() + "\n" + m.renderBotDivider() + m.footerSuffix
}

// cycleSort toggles direction if key matches current sort, or sets a new sort key (descending).
func (m Model) cycleSort(key SortKey) Model {
	if m.sortKey == key {
		m.sortDesc = !m.sortDesc
	} else {
		m.sortKey = key
		m.sortDesc = true
	}
	return m
}

// updatePicker handles key events when the namespace picker is open.
// Printable characters go to the filter query; arrow keys navigate the filtered list.
func (m Model) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rebuild := func() Model {
		if m.ready && m.pods != nil {
			return m.rebuildViewport()
		}
		return m
	}

	if msg.String() == "*" {
		m.pickerQuery = ""
		m.pickerCursor = 0 // allNamespacesEntry is always index 0 when query is empty
		m = rebuild()
		return m, nil
	}

	switch msg.Type {
	case tea.KeyRunes:
		m.pickerQuery += string(msg.Runes)
		m.pickerCursor = 0
		m = rebuild()
		return m, nil
	case tea.KeyBackspace:
		if len(m.pickerQuery) > 0 {
			runes := []rune(m.pickerQuery)
			m.pickerQuery = string(runes[:len(runes)-1])
			m.pickerCursor = 0
			m = rebuild()
		}
		return m, nil
	case tea.KeyCtrlU:
		if len(m.pickerQuery) > 0 {
			m.pickerQuery = ""
			m.pickerCursor = 0
			m = rebuild()
		}
		return m, nil
	case tea.KeyUp:
		if m.pickerCursor > 0 {
			m.pickerCursor--
			m = rebuild()
		}
		return m, nil
	case tea.KeyDown:
		display := buildDisplayList(filteredNamespaces(m.namespaces, m.pickerQuery), m.pickerQuery)
		if m.pickerCursor < len(display)-1 {
			m.pickerCursor++
			m = rebuild()
		}
		return m, nil
	case tea.KeyEnter:
		display := buildDisplayList(filteredNamespaces(m.namespaces, m.pickerQuery), m.pickerQuery)
		if len(display) > 0 && m.pickerCursor < len(display) {
			selected := display[m.pickerCursor]
			if selected == allNamespacesEntry {
				m.allNamespaces = true
				m.namespace = ""
			} else {
				m.namespace = selected
				m.allNamespaces = false
			}
			m.pickerMode = false
			m.pickerQuery = ""
			m = rebuild()
			return m, m.fetchCmd()
		}
		return m, nil
	case tea.KeyEsc:
		m.pickerMode = false
		m.pickerQuery = ""
		m = rebuild()
		return m, nil
	case tea.KeyCtrlC:
		return m, tea.Quit
	}
	return m, nil
}

// filteredNamespaces returns the subset of namespaces whose names contain query
// (case-insensitive). When query is empty the original slice is returned unchanged.
func filteredNamespaces(namespaces []string, query string) []string {
	if query == "" {
		return namespaces
	}
	q := strings.ToLower(query)
	out := make([]string, 0, len(namespaces))
	for _, ns := range namespaces {
		if strings.Contains(strings.ToLower(ns), q) {
			out = append(out, ns)
		}
	}
	return out
}

// buildDisplayList prepends the allNamespacesEntry sentinel when the query is empty
// so users can always navigate back to all-namespaces mode from the picker.
func buildDisplayList(filtered []string, query string) []string {
	if query != "" {
		return filtered
	}
	display := make([]string, 0, len(filtered)+1)
	display = append(display, allNamespacesEntry)
	display = append(display, filtered...)
	return display
}

// rebuildViewport recomputes header/footer content and sizes the viewport to the
// actual pod rows, with the TOTAL area pinned just below and the footer floated
// to the bottom of the terminal via variable padding.
//
// View() assembles the final output as:
//
//	headerBase + renderTopDivider() + viewport.View() + "\n" + renderBotDivider() + footerSuffix
//
// The two thick dividers are rendered dynamically so they can carry live scroll
// indicators (▲ above / ▼ more) without rebuilding the entire layout on each scroll.
func (m Model) rebuildViewport() Model {
	sorted := sortPods(m.pods, m.metrics, m.sortKey, m.sortDesc)

	lay := newLayout(sorted, m.wide, m.allNamespaces)
	m.tableWidth = lay.totalWidth()

	m.headerBase = RenderFixedHeaderBase(m.displayNamespace(), m.cluster, m.user, m.selector, sorted, m.metrics, m.quota, m.thresh, m.styles, m.wide, m.allNamespaces, m.sortKey, m.sortDesc, m.termWidth)

	totals := computeTotals(sorted, m.metrics)
	totalsRow := renderTotalsRow(totals, m.styles, lay)

	var footerBody string
	if m.pickerMode {
		filtered := filteredNamespaces(m.namespaces, m.pickerQuery)
		display := buildDisplayList(filtered, m.pickerQuery)
		footerBody = renderNamespacePicker(display, m.pickerCursor, m.pickerLoading, m.pickerQuery, m.styles, m.termWidth)
	} else {
		footerBody = renderWatchFooterBody(sorted, m.styles, m.wide, m.allNamespaces, m.sortKey, m.sortDesc, m.termWidth)
	}
	podBody := RenderBody(sorted, m.metrics, m.thresh, m.styles, m.podDividers, m.wide, m.allNamespaces)

	// headerLines: headerBase line count + 1 for the top thick divider rendered by View().
	headerLines := strings.Count(m.headerBase, "\n") + 1
	// totalAreaLines: 1 for the bottom thick divider (rendered by View()) + 1 for the TOTAL row.
	const totalAreaLines = 2
	// blankSeparatorLines: the blank row always shown between the TOTAL row and the footer.
	const blankSeparatorLines = 1
	footerBodyLines := strings.Count(footerBody, "\n")
	// Count lines before trimming so the height calculation is correct.
	podBodyLines := strings.Count(podBody, "\n")
	// Trim the trailing newline so the viewport doesn't see a phantom empty line
	// (which would cause "▼ more" to appear even when all pods are visible).
	podBodyContent := strings.TrimSuffix(podBody, "\n")

	// The explicit "\n" in View() terminates the viewport's last rendered line.
	// maxVP is the largest the viewport can be while keeping everything on screen.
	// vpHeight is capped at the actual pod row count so the TOTAL row always sits
	// immediately beneath the last pod row (viewport expands only when scrolling is needed).
	maxVP := max(1, m.termHeight-headerLines-1-totalAreaLines-blankSeparatorLines-footerBodyLines)
	vpHeight := max(1, min(podBodyLines, maxVP))

	m.viewport.Width = m.termWidth
	m.viewport.Height = vpHeight
	m.viewport.SetContent(podBodyContent)
	// If all pods now fit in the viewport (no scrolling needed), reset to the top
	// so rows that were previously hidden above become visible after a window resize.
	if vpHeight >= podBodyLines {
		m.viewport.GotoTop()
	}

	// footerSuffix: TOTAL row, one blank separator line, then the keyboard hint / picker.
	m.footerSuffix = totalsRow + "\n" + "\n" + footerBody
	return m
}

// fetchNamespacesCmd returns a tea.Cmd that fetches the namespace list in the background.
func (m Model) fetchNamespacesCmd() tea.Cmd {
	return func() tea.Msg {
		namespaces, err := m.client.ListNamespaces()
		return namespacesResult{namespaces: namespaces, err: err}
	}
}

// fetchCmd returns a tea.Cmd that fetches pods, metrics, and quota in the background.
func (m Model) fetchCmd() tea.Cmd {
	return func() tea.Msg {
		pods, err := m.client.ListPods(m.namespace, m.selector)
		if err != nil {
			return fetchResult{err: err}
		}
		metrics, err := m.client.FetchMetrics(m.namespace)
		if err != nil {
			return fetchResult{err: err}
		}
		// Quota is per-namespace and not meaningful in all-namespaces mode.
		var quota *kube.NamespaceQuota
		if !m.allNamespaces {
			quota, err = m.client.GetResourceQuota(m.namespace)
			if err != nil {
				return fetchResult{err: err}
			}
		}
		return fetchResult{pods: pods, metrics: metrics, quota: quota}
	}
}
