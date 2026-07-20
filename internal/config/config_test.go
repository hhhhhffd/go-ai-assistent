package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsYAMLAndBootstrapEnvironment(t *testing.T) {
	t.Setenv(ConfigDirEnv, "")
	directory := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	content := "telegram:\n  enabled: true\n  allowed_user_ids: [42]\ndiscord:\n  enabled: false\nai:\n  request_timeout: 20s\n"
	if err := os.WriteFile(filepath.Join(directory, DefaultConfigPath), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{"TELEGRAM_API_ID": "1", "TELEGRAM_API_HASH": "hash", "TELEGRAM_BOT_TOKEN": "token", "OWNER_TELEGRAM_ID": "7"} {
		t.Setenv(key, value)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Telegram.Enabled || cfg.Telegram.OwnerID != 7 {
		t.Fatalf("unexpected telegram config: %#v", cfg.Telegram)
	}
	if _, ok := cfg.Telegram.AllowedUserIDs[42]; !ok {
		t.Fatal("yaml allowlist entry is missing")
	}
	if _, err := os.Stat(DefaultProvidersPath); err != nil {
		t.Fatalf("providers file was not created: %v", err)
	}
}

func TestSaveProvidersWritesPrivateFile(t *testing.T) {
	t.Setenv(ConfigDirEnv, "")
	directory := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	if err := SaveProviders(ProvidersConfig{Primary: ProviderConfig{Name: "test", APIKey: "secret"}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(DefaultProvidersPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600, got %o", info.Mode().Perm())
	}
}

func TestConfigDirStoresAllStateFiles(t *testing.T) {
	directory := t.TempDir()
	t.Setenv(ConfigDirEnv, directory)

	if err := EnsureFiles(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(directory, DefaultConfigPath), filepath.Join(directory, DefaultProvidersPath)} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("state file %s: %v", path, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("state file %s mode = %o, want 600", path, info.Mode().Perm())
		}
	}
}

func TestParseAllowLists(t *testing.T) {
	if _, err := parseTelegramAllowlist("1,2"); err != nil {
		t.Fatal(err)
	}
	if _, err := parseDiscordAllowlist("1,2"); err != nil {
		t.Fatal(err)
	}
}
