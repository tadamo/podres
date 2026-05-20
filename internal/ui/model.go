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
	client      *kube.Client
	namespace   string
	selector    string
	cluster     string
	user        string
	thresh      threshold.Config
	styles      Styles
	interval    time.Duration
	noWatch     bool
	podDividers bool
	wide        bool

	// sort state
	sortKey  SortKey
	sortDesc bool

	// current display state
	pods    []kube.PodSpec
	metrics map[string]kube.PodMetrics
	quota   *kube.NamespaceQuota
	err     error

	// viewport for the scrollable table body (watch mode only)
	viewport      viewport.Model
	ready         bool
	termWidth     int
	termHeight    int
	headerContent string
	footerContent string
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
	initialSort SortKey,
) Model {
	// CPU/mem/restarts default to descending (highest first); name defaults to ascending.
	desc := initialSort == SortCPU || initialSort == SortMem || initialSort == SortRestarts
	return Model{
		client:      client,
		namespace:   namespace,
		selector:    selector,
		cluster:     cluster,
		user:        user,
		thresh:      thresh,
		styles:      styles,
		interval:    interval,
		noWatch:     noWatch,
		podDividers: podDividers,
		wide:        wide,
		sortKey:     initialSort,
		sortDesc:    desc,
	}
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
		case "n":
			m = m.cycleSort(SortName)
			if m.ready && m.pods != nil {
				m = m.rebuildViewport()
			}
			return m, nil
		case "0":
			m.sortKey = SortNone
			m.sortDesc = false
			if m.ready && m.pods != nil {
				m = m.rebuildViewport()
			}
			return m, nil
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
		return Render(m.namespace, m.cluster, m.user, m.selector, sorted, m.metrics, m.quota, m.thresh, m.styles, m.podDividers, m.wide, m.sortKey, m.sortDesc)
	}
	if !m.ready {
		return "Loading…\n"
	}
	return m.headerContent + m.viewport.View() + m.footerContent
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

// rebuildViewport recomputes header/footer content and resizes the viewport to
// fill the remaining terminal height between them.
func (m Model) rebuildViewport() Model {
	sorted := sortPods(m.pods, m.metrics, m.sortKey, m.sortDesc)
	m.headerContent = RenderFixedHeader(m.namespace, m.cluster, m.user, m.selector, sorted, m.metrics, m.quota, m.thresh, m.styles, m.wide, m.sortKey, m.sortDesc)
	m.footerContent = RenderFixedFooter(sorted, m.metrics, m.thresh, m.styles, m.wide, m.sortKey, m.sortDesc)
	headerLines := strings.Count(m.headerContent, "\n")
	footerLines := strings.Count(m.footerContent, "\n")
	m.viewport.Width = m.termWidth
	m.viewport.Height = max(1, m.termHeight-headerLines-footerLines)
	m.viewport.SetContent(RenderBody(sorted, m.metrics, m.thresh, m.styles, m.podDividers, m.wide))
	return m
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
		quota, err := m.client.GetResourceQuota(m.namespace)
		if err != nil {
			return fetchResult{err: err}
		}
		return fetchResult{pods: pods, metrics: metrics, quota: quota}
	}
}
