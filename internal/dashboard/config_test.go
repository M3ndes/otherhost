package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigTreatsValuesAsData(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(t.TempDir(), "otherhost.conf")
	sideEffect := filepath.Join(t.TempDir(), "must-not-exist")
	content := strings.Join([]string{
		"otherhost_name=test-box",
		"host=192.0.2.10",
		"ssh_user=developer",
		"ssh_port=2222",
		"identity_file=.ssh/otherhost key",
		"known_hosts_file=.ssh/otherhost known hosts",
		"projects_root=~/src/$(touch " + sideEffect + ")",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig returned an error: %v", err)
	}
	if config.Name != "test-box" || config.Host != "192.0.2.10" || config.SSHUser != "developer" {
		t.Fatalf("unexpected config: %#v", config)
	}
	if config.IdentityFile != filepath.Join(home, ".ssh/otherhost key") {
		t.Fatalf("identity path was not resolved: %q", config.IdentityFile)
	}
	if _, err := os.Stat(sideEffect); !os.IsNotExist(err) {
		t.Fatalf("configuration value was executed")
	}
}

func TestLoadConfigRejectsCommandLikeHost(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "otherhost.conf")
	content := "otherhost_name=test-box\nhost=host;reboot\nssh_user=developer\nssh_port=2222\nidentity_file=.ssh/key\n"
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(configPath); err == nil {
		t.Fatal("unsafe host value was accepted")
	}
}

func TestLoadConfigAcceptsLegacyMachineName(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "devbox.local.conf")
	content := "devbox_name=legacy-box\nhost=192.0.2.10\nssh_user=developer\nssh_port=2222\nidentity_file=.ssh/key\n"
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("legacy config was not accepted: %v", err)
	}
	if config.Name != "legacy-box" {
		t.Fatalf("unexpected legacy machine name: %q", config.Name)
	}
}

func TestNormalizeProjectsRootStaysInsideRemoteHome(t *testing.T) {
	for _, value := range []string{"/src", "~/../src", "~/src/../secrets", "~/"} {
		if _, err := normalizeProjectsRoot(value); err == nil {
			t.Fatalf("unsafe projects root was accepted: %q", value)
		}
	}
	if root, err := normalizeProjectsRoot(""); err != nil || root != "src" {
		t.Fatalf("default root was not src: %q, %v", root, err)
	}
}
