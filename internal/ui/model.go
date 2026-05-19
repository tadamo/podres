package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tadamo/podres/internal/kube"
	"github.com/tadamo/podres/internal/threshold"
)

// Model is the Bubbletea application model for watch-mode display.
type Model struct {
	client    *kube.Client
	namespace string
	thresh    threshold.Config
	styles    Styles
	interval  time.Duration
	noWatch   bool

	// current display state
	pods    []kube.PodSpec
	metrics map[string]kube.PodMetrics
	err     error
}

// fetchResult carries the outcome of one refresh cycle.
type fetchResult struct {
	pods    []kube.PodSpec
	metrics map[string]kube.PodMetrics
	err     error
}

// tickMsg fires when the refresh interval elapses.
type tickMsg struct{}

// New returns an initialized Model ready to run.
func New(
	client *kube.Client,
	namespace string,
	thresh threshold.Config,
	styles Styles,
	interval time.Duration,
	noWatch bool,
) Model {
	return Model{
		client:    client,
		namespace: namespace,
		thresh:    thresh,
		styles:    styles,
		interval:  interval,
		noWatch:   noWatch,
	}
}

// Init triggers the first data fetch immediately on startup.
func (m Model) Init() tea.Cmd {
	return m.fetchCmd()
}

// Update handles incoming messages and drives state transitions.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case fetchResult:
		m.pods = msg.pods
		m.metrics = msg.metrics
		m.err = msg.err

		if m.noWatch {
			return m, tea.Quit
		}
		// Schedule the next tick; the tick handler will trigger the fetch.
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
	return Render(m.namespace, m.pods, m.metrics, m.thresh, m.styles)
}

// fetchCmd returns a tea.Cmd that fetches pods and metrics in the background.
func (m Model) fetchCmd() tea.Cmd {
	return func() tea.Msg {
		pods, err := m.client.ListPods(m.namespace)
		if err != nil {
			return fetchResult{err: err}
		}
		metrics, err := m.client.FetchMetrics(m.namespace)
		if err != nil {
			return fetchResult{err: err}
		}
		return fetchResult{pods: pods, metrics: metrics}
	}
}
