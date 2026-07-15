package gitlab

import (
	"os"
	"path/filepath"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// AuthConfig holds everything needed to talk to a GitLab instance.
// It is resolved from (in order of precedence) environment variables and the
// glab CLI config file (~/.config/glab-cli/config.yml), so gl-pipeline works
// out of the box for anyone who already uses glab, without depending on it.
type AuthConfig struct {
	Host        string
	Token       string
	APIProtocol string
	IsJobToken  bool
}

// LoadAuthConfig resolves the GitLab authentication configuration.
//
// Host resolution: GITLAB_HOST env var, else the glab config, else gitlab.com.
// Token resolution: GITLAB_TOKEN env var, else the glab config token, else the
// CI_JOB_TOKEN env var (which flags the client to use the job-token header).
func LoadAuthConfig() (AuthConfig, error) {
	host := "gitlab.com"
	if v := os.Getenv("GITLAB_HOST"); v != "" {
		host = v
	}

	cfg, _ := loadFromFile(defaultConfigPath(), host)
	cfg.Host = host
	if cfg.APIProtocol == "" {
		cfg.APIProtocol = "https"
	}

	if tok := os.Getenv("GITLAB_TOKEN"); tok != "" {
		cfg.Token = tok
	} else if cfg.Token == "" {
		if tok := os.Getenv("CI_JOB_TOKEN"); tok != "" {
			cfg.Token = tok
			cfg.IsJobToken = true
		}
	}

	return cfg, nil
}

func defaultConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "glab-cli", "config.yml")
}

func loadFromFile(path, host string) (AuthConfig, error) {
	k := koanf.New(".")
	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return AuthConfig{}, err
	}
	return AuthConfig{
		Token:       k.String("hosts." + host + ".token"),
		APIProtocol: k.String("hosts." + host + ".api_protocol"),
	}, nil
}
