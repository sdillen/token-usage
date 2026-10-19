package claude

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCredentials(t *testing.T) {
	const fileCreds = `{"claudeAiOauth":{"accessToken":"file"}}`
	const keychainCreds = `{"claudeAiOauth":{"accessToken":"keychain"}}`

	tests := []struct {
		name        string
		custom      bool   // dir is a custom config dir instead of ~/.claude
		file        bool   // credentials file exists
		goos        string // platform passed to loadCredentials
		keychainErr bool   // Keychain lookup fails
		wantToken   string // empty means an error is expected
	}{
		{name: "file wins on darwin", file: true, goos: "darwin", wantToken: "file"},
		{name: "keychain fallback on darwin", goos: "darwin", wantToken: "keychain"},
		{name: "keychain error", goos: "darwin", keychainErr: true},
		{name: "no keychain for custom dir", custom: true, goos: "darwin"},
		{name: "no keychain off darwin", goos: "linux"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, ".claude")
			if tc.custom {
				dir = t.TempDir()
			}
			if tc.file {
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(fileCreds), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			keychain := func() ([]byte, error) {
				if tc.keychainErr {
					return nil, errors.New("not found")
				}
				return []byte(keychainCreds), nil
			}

			got, err := loadCredentials(dir, home, tc.goos, keychain)
			if tc.wantToken == "" {
				if err == nil {
					t.Fatalf("want error, got token %q", got.AccessToken)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.AccessToken != tc.wantToken {
				t.Errorf("token = %q, want %q", got.AccessToken, tc.wantToken)
			}
		})
	}
}
