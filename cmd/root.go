package cmd

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/tadamo/podres/internal/kube"
	"github.com/tadamo/podres/internal/threshold"
	"github.com/tadamo/podres/internal/ui"
)

// Options holds all CLI flag values.
type Options struct {
	Namespace      string
	Selector       string
	Interval       time.Duration
	NoWatch        bool
	Kubeconfig     string
	Context        string
	ThresholdWarn  int
	ThresholdCrit  int
	NoColor        bool
	PodDividers    bool
}

var rootCmd = &cobra.Command{
	Use:   "kubectl-podres",
	Short: "Watch pod resource requests, limits, and live utilization",
	Long: `podres displays a real-time, colorized view of Kubernetes pod and container
resource requests, limits, and live utilization in a single compact table.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts, err := optionsFromFlags(cmd)
		if err != nil {
			return err
		}
		return runPodres(opts)
	},
}

// Execute is the entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	f := rootCmd.Flags()
	f.StringP("namespace", "n", "", "namespace to watch (defaults to current context namespace)")
	f.StringP("selector", "l", "", "label selector to filter pods (e.g. app=nginx)")
	f.Duration("interval", 5*time.Second, "refresh interval in watch mode")
	f.Bool("no-watch", false, "print once and exit")
	f.String("kubeconfig", "", "path to kubeconfig (defaults to ~/.kube/config)")
	f.String("context", "", "kubeconfig context to use (defaults to current context)")
	f.Int("threshold-warn", 75, "yellow warning threshold (percent)")
	f.Int("threshold-crit", 95, "red critical threshold (percent)")
	f.Bool("no-color", false, "disable colorized output")
	f.Bool("pod-dividers", false, "draw a horizontal rule between each pod")
}

func optionsFromFlags(cmd *cobra.Command) (Options, error) {
	f := cmd.Flags()
	namespace, _ := f.GetString("namespace")
	selector, _ := f.GetString("selector")
	interval, _ := f.GetDuration("interval")
	noWatch, _ := f.GetBool("no-watch")
	kubeconfig, _ := f.GetString("kubeconfig")
	context, _ := f.GetString("context")
	warnPct, _ := f.GetInt("threshold-warn")
	critPct, _ := f.GetInt("threshold-crit")
	noColor, _ := f.GetBool("no-color")
	podDividers, _ := f.GetBool("pod-dividers")

	if warnPct >= critPct {
		return Options{}, fmt.Errorf("--threshold-warn (%d) must be less than --threshold-crit (%d)", warnPct, critPct)
	}

	return Options{
		Namespace:     namespace,
		Selector:      selector,
		Interval:      interval,
		NoWatch:       noWatch,
		Kubeconfig:    kubeconfig,
		Context:       context,
		ThresholdWarn: warnPct,
		ThresholdCrit: critPct,
		NoColor:       noColor,
		PodDividers:   podDividers,
	}, nil
}

func runPodres(opts Options) error {
	client, err := kube.New(opts.Kubeconfig, opts.Context)
	if err != nil {
		return fmt.Errorf("connect to cluster: %w", err)
	}

	namespace := opts.Namespace
	if namespace == "" {
		namespace, err = client.CurrentNamespace()
		if err != nil {
			return fmt.Errorf("resolve namespace: %w", err)
		}
	}

	cluster, user, err := client.ClusterInfo()
	if err != nil {
		return fmt.Errorf("resolve cluster info: %w", err)
	}

	thresh := threshold.Config{
		Warn: opts.ThresholdWarn,
		Crit: opts.ThresholdCrit,
	}
	styles := ui.DefaultStyles(opts.NoColor)
	model := ui.New(client, namespace, opts.Selector, cluster, user, thresh, styles, opts.Interval, opts.NoWatch, opts.PodDividers)

	var progOpts []tea.ProgramOption
	if !opts.NoWatch {
		progOpts = append(progOpts, tea.WithAltScreen())
	}

	_, err = tea.NewProgram(model, progOpts...).Run()
	return err
}
