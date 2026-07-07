package calendar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecret(t *testing.T) {
	cases := []struct {
		name            string
		account         Account
		credentialsFile string
		permissions     os.FileMode
		want            string
		wantErr         string
	}{
		{
			name:    "credential command output is trimmed",
			account: Account{Name: "work", CredentialCommand: "echo '  app-password  '"},
			want:    "app-password",
		},
		{
			name:    "credential command failure is reported",
			account: Account{Name: "work", CredentialCommand: "exit 1"},
			wantErr: "credential_command failed",
		},
		{
			name:    "credential command with empty output is reported",
			account: Account{Name: "work", CredentialCommand: "true"},
			wantErr: "produced no output",
		},
		{
			name:            "credentials file entry",
			account:         Account{Name: "work"},
			credentialsFile: "[work]\nsecret = \"file-password\"\n",
			permissions:     0o600,
			want:            "file-password",
		},
		{
			name:            "credentials file readable by others is refused",
			account:         Account{Name: "work"},
			credentialsFile: "[work]\nsecret = \"file-password\"\n",
			permissions:     0o644,
			wantErr:         "readable by other users",
		},
		{
			name:            "missing account entry is reported",
			account:         Account{Name: "home"},
			credentialsFile: "[work]\nsecret = \"file-password\"\n",
			permissions:     0o600,
			wantErr:         "has no secret",
		},
		{
			name:    "missing credentials file is reported",
			account: Account{Name: "work"},
			wantErr: "has no secret",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "credentials.toml")
			t.Setenv("CALTUI_CREDENTIALS", path)

			if c.credentialsFile != "" {
				if err := os.WriteFile(path, []byte(c.credentialsFile), c.permissions); err != nil {
					t.Fatal(err)
				}
			}

			secret, err := c.account.Secret()

			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("want error containing %q, got %v", c.wantErr, err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if secret != c.want {
				t.Fatalf("want secret %q, got %q", c.want, secret)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		account Account
		wantErr string
	}{
		{
			name:    "valid caldav account",
			account: Account{Name: "work", Type: "caldav", URL: "https://dav.example.com", Username: "raj"},
		},
		{
			name:    "valid ics account",
			account: Account{Name: "google", Type: "ics"},
		},
		{
			name:    "account name with slash is refused",
			account: Account{Name: "../escape", Type: "ics"},
			wantErr: "letters, digits, dashes and underscores",
		},
		{
			name:    "unknown type is refused",
			account: Account{Name: "work", Type: "exchange"},
			wantErr: "type must be",
		},
		{
			name:    "caldav without url is refused",
			account: Account{Name: "work", Type: "caldav", Username: "raj"},
			wantErr: "need a url",
		},
		{
			name:    "caldav without username is refused",
			account: Account{Name: "work", Type: "caldav", URL: "https://dav.example.com"},
			wantErr: "need a username",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.account.Validate()

			if c.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}

				return
			}

			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("want error containing %q, got %v", c.wantErr, err)
			}
		})
	}
}
