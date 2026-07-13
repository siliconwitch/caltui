package calendar

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

const AccountsSection = "accounts"

type Account struct {
	Name              string `toml:"name"`
	Type              string `toml:"type"`
	URL               string `toml:"url"`
	Username          string `toml:"username"`
	Email             string `toml:"email"`
	CredentialCommand string `toml:"credential_command"`
}

var accountNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func (a Account) validate() error {
	if !accountNamePattern.MatchString(a.Name) {
		return fmt.Errorf("account name %q must contain only letters, digits, dashes and underscores", a.Name)
	}

	switch a.Type {
	case "caldav":
		if a.URL == "" {
			return fmt.Errorf("account %q: caldav accounts need a url, in the config or in %s", a.Name, credentialsPath())
		}

		if a.Username == "" {
			return fmt.Errorf("account %q: caldav accounts need a username, in the config or in %s", a.Name, credentialsPath())
		}

		return nil

	case "ics":
		return nil

	default:
		return fmt.Errorf("account %q: type must be \"caldav\" or \"ics\", not %q", a.Name, a.Type)
	}
}

type credentialsEntry struct {
	Secret   string `toml:"secret"`
	Username string `toml:"username"`
	URL      string `toml:"url"`
}

func storedCredentials(accountName string) (credentialsEntry, error) {
	path := credentialsPath()

	info, err := os.Stat(path)

	if err != nil {
		return credentialsEntry{}, nil
	}

	if info.Mode().Perm()&0o077 != 0 {
		return credentialsEntry{}, fmt.Errorf("%s is readable by other users: run chmod 600 %s", path, path)
	}

	var entries map[string]credentialsEntry

	_, err = toml.DecodeFile(path, &entries)

	if err != nil {
		return credentialsEntry{}, fmt.Errorf("reading %s: %w", path, err)
	}

	return entries[accountName], nil
}

func (a Account) secret() (string, error) {
	if a.CredentialCommand != "" {
		output, err := exec.Command("sh", "-c", a.CredentialCommand).Output()

		if err != nil {
			return "", fmt.Errorf("account %q: credential_command failed: %w", a.Name, err)
		}

		secret := strings.TrimSpace(string(output))
		if secret == "" {
			return "", fmt.Errorf("account %q: credential_command produced no output", a.Name)
		}

		return secret, nil
	}

	entry, err := storedCredentials(a.Name)

	if err != nil {
		return "", err
	}

	if entry.Secret == "" {
		return "", fmt.Errorf(
			"account %q has no secret: set credential_command in the config, or put secret = \"...\" in a [%s] section of %s",
			a.Name, a.Name, credentialsPath(),
		)
	}

	return entry.Secret, nil
}

func credentialsPath() string {
	if path := os.Getenv("CALTUI_CREDENTIALS"); path != "" {
		return path
	}

	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, err := os.UserHomeDir()

		if err != nil {
			home = "."
		}

		stateHome = filepath.Join(home, ".local", "state")
	}

	return filepath.Join(stateHome, "caltui", "credentials.toml")
}
