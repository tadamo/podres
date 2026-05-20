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
) Model {
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
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
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
		return Render(m.namespace, m.cluster, m.user, m.selector, m.pods, m.metrics, m.quota, m.thresh, m.styles, m.podDividers, m.wide)
	}
	if !m.ready {
		return "Loading…\n"
	}
	return m.headerContent + m.viewport.View() + m.footerContent
}

// rebuildViewport recomputes header/footer content and resizes the viewport to
// fill the remaining terminal height between them.
func (m Model) rebuildViewport() Model {
	m.headerContent = RenderFixedHeader(m.namespace, m.cluster, m.user, m.selector, m.pods, m.metrics, m.quota, m.thresh, m.styles, m.wide)
	m.footerContent = RenderFixedFooter(m.pods, m.metrics, m.thresh, m.styles)
	headerLines := strings.Count(m.headerContent, "\n")
	footerLines := strings.Count(m.footerContent, "\n")
	m.viewport.Width = m.termWidth
	m.viewport.Height = max(1, m.termHeight-headerLines-footerLines)
	m.viewport.SetContent(RenderBody(m.pods, m.metrics, m.thresh, m.styles, m.podDividers, m.wide))
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
