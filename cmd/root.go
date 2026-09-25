package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"github.com/iamnikolie/exa-cli/internal/client"
	"github.com/iamnikolie/exa-cli/internal/config"
	"github.com/spf13/cobra"
)

// version is stamped at link time by the Makefile and by GoReleaser
// (-X github.com/iamnikolie/exa-cli/cmd.version=…). An unstamped build says
// "dev" rather than claiming a version it does not have; buildVersion appends
// the VCS revision when the toolchain embedded one.
var version = "dev"

func buildVersion() string {
	v := version
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				rev := s.Value
				if len(rev) > 12 {
					rev = rev[:12]
				}
				v += " (" + rev + ")"
			}
		}
	}
	return v
}

var (
	jsonOutput   bool
	profile      string
	verbose      bool
	outputFormat string
	fieldsFlag   []string
	timeout      time.Duration

	cfg *config.Config
	cli *client.Client
)

var (
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
	stdin  io.Reader = os.Stdin
)

var rootCmd = &cobra.Command{
	Use:   "exa",
	Short: "Exa CLI — web search, contents, answers and agent runs from the terminal",
	Long: "Exa CLI — web search, contents, answers and agent runs from the terminal.\n\n" +
		"Run 'exa skill' to print the full agent reference (commands, flags, workflows).",
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		switch outputFormat {
		case "", "md", "table", "json", "csv", "tsv":
		default:
			return fmt.Errorf("--format must be one of md, table, json, csv, tsv")
		}
		if profileExempt(cmd) {
			return nil
		}
		var err error
		cfg, err = config.Load(profile)
		if err != nil {
			return err
		}
		if err := cfg.Validate(); err != nil {
			return err
		}
		client.UserAgent = "exa-cli/" + version
		cli = client.New(cfg.APIKey, cfg.BaseURL)
		cli.Verbose = verbose
		return nil
	},
}

func Execute() {
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		os.Exit(1)
	}
}

func init() {
	def := os.Getenv("EXA_CONFIG")
	if def == "" {
		def = config.DefaultProfile
	}
	pf := rootCmd.PersistentFlags()
	pf.BoolVar(&jsonOutput, "json", false, "output the raw API JSON (same as --format json)")
	pf.StringVar(&profile, "config", def, "profile name (subdirectory of ~/.exa-cli/; env EXA_CONFIG)")
	pf.BoolVar(&verbose, "verbose", false, "dump API requests and responses to stderr")
	pf.StringVar(&outputFormat, "format", "", "output format: md (default), table, json, csv, tsv")
	pf.StringSliceVar(&fieldsFlag, "fields", nil, "comma-separated output fields (dotted paths allowed), e.g. url,title")
	pf.DurationVar(&timeout, "timeout", 5*time.Minute, "per-request timeout (0 disables)")

	rootCmd.Version = buildVersion()
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show the exa version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintf(stdout, "exa version %s\n", buildVersion())
		return nil
	},
}

// profileExempt reports whether a command runs without an API key.
func profileExempt(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "exa", "skill", "help", "completion", "version", "bash", "zsh", "fish", "powershell":
		return true
	}
	return cmd.Parent() != nil && cmd.Parent().Name() == "config"
}

// reqCtx bounds a single API request by --timeout.
func reqCtx(cmd *cobra.Command) (context.Context, context.CancelFunc) {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}
