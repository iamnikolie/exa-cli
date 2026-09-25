package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iamnikolie/exa-cli/internal/config"
	"github.com/spf13/cobra"
)

var initKey string

func maskKey(k string) string {
	if k == "" {
		return "not set"
	}
	if len(k) <= 8 {
		return "set"
	}
	return "set (…" + k[len(k)-4:] + ")"
}

// resolveInitKey picks the key: --api-key flag, else EXA_API_KEY, else the
// first non-empty line from r.
func resolveInitKey(flag string, r io.Reader) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if env := os.Getenv("EXA_API_KEY"); env != "" {
		return env, nil
	}
	line, _ := bufio.NewReader(r).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return "", fmt.Errorf("no API key provided: pass --api-key, set EXA_API_KEY, or pipe it on stdin")
	}
	return line, nil
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage the API key and profiles",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Save an API key (writes ~/.exa-cli/<profile>/config.yaml, mode 0600)",
	Example: `  exa config init                         # prompts; paste the key
  pbpaste | exa config init
  exa --config work config init --api-key "$WORK_EXA_KEY"`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if initKey == "" && os.Getenv("EXA_API_KEY") == "" {
			fmt.Fprint(stderr, "Exa API key (https://dashboard.exa.ai/api-keys): ")
		}
		key, err := resolveInitKey(initKey, stdin)
		if err != nil {
			return err
		}
		path, err := config.Save(key, profile)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Saved to %s\n", path)
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the active profile, base URL and key state",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := config.Load(profile)
		if err != nil {
			return err
		}
		path, _ := config.Path(profile)
		source := "file"
		if os.Getenv("EXA_API_KEY") != "" {
			source = "EXA_API_KEY"
		}
		row := map[string]any{
			"profile":  profile,
			"file":     path,
			"base_url": c.BaseURL,
			"api_key":  maskKey(c.APIKey),
			"source":   source,
		}
		cols := []string{"profile", "file", "base_url", "api_key", "source"}
		if format() == "json" {
			return writeJSON(stdout, row)
		}
		return emitRows([]map[string]any{row}, cols)
	},
}

func init() {
	configInitCmd.Flags().StringVar(&initKey, "api-key", "", "Exa API key; else EXA_API_KEY or stdin")
	configCmd.AddCommand(configInitCmd, configShowCmd)
	rootCmd.AddCommand(configCmd)
}
