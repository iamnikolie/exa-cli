package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultBaseURL is the Exa API root. EXA_BASE_URL overrides it (proxies, tests).
const DefaultBaseURL = "https://api.exa.ai"

// DefaultProfile is used when neither --config nor EXA_CONFIG names one.
const DefaultProfile = "default"

type Config struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url,omitempty"`
}

// Validate errors when no API key is configured.
func (c *Config) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("API key not set: run 'exa config init' or set EXA_API_KEY (get a key at https://dashboard.exa.ai/api-keys)")
	}
	return nil
}

// home returns the config dir for a profile. EXA_CLI_HOME overrides the base
// (test escape hatch); the default base is ~/.exa-cli.
func home(profile string) (string, error) {
	base := os.Getenv("EXA_CLI_HOME")
	if base == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(h, ".exa-cli")
	}
	if profile == "" {
		profile = DefaultProfile
	}
	return filepath.Join(base, profile), nil
}

// Path returns the config file path for a profile.
func Path(profile string) (string, error) {
	dir, err := home(profile)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Load reads the profile's config file. EXA_API_KEY and EXA_BASE_URL override
// the file values; a missing file is not an error.
func Load(profile string) (*Config, error) {
	cfg := &Config{}
	path, err := Path(profile)
	if err != nil {
		return nil, fmt.Errorf("config.Load: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("config.Load: %w", err)
	}
	if err == nil {
		if e := yaml.Unmarshal(data, cfg); e != nil {
			return nil, fmt.Errorf("config.Load: parse %s: %w", path, e)
		}
	}
	if env := os.Getenv("EXA_API_KEY"); env != "" {
		cfg.APIKey = env
	}
	if env := os.Getenv("EXA_BASE_URL"); env != "" {
		cfg.BaseURL = env
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	return cfg, nil
}

// Save writes the key to ~/.exa-cli/<profile>/config.yaml (dir 0700, file 0600).
func Save(apiKey, profile string) (string, error) {
	path, err := Path(profile)
	if err != nil {
		return "", fmt.Errorf("config.Save: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("config.Save: mkdir: %w", err)
	}
	data, err := yaml.Marshal(Config{APIKey: apiKey})
	if err != nil {
		return "", fmt.Errorf("config.Save: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("config.Save: %w", err)
	}
	return path, nil
}
