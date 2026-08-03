package dashboard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestClientConnectionManagerPersistsPauseAndReconnect(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "nested", "connection-state")
	manager := NewClientConnectionManager(Config{Name: "test-host", Host: "192.0.2.10", SSHUser: "developer", SSHPort: 2222, KnownHostsFile: "/tmp/known-hosts"}, statePath)
	manager.probe = func(context.Context, Config) error { return nil }

	if status := manager.Status(); status.State != connectionStateConnected || !status.Paired || !status.IdentityPinned {
		t.Fatalf("unexpected initial status: %+v", status)
	}
	if err := manager.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if status := manager.Status(); status.State != connectionStateDisconnected {
		t.Fatalf("unexpected paused status: %+v", status)
	}
	if info, err := os.Stat(statePath); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("connection state permissions are unsafe: info=%v err=%v", info, err)
	}
	if err := manager.Reconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if status := manager.Status(); status.State != connectionStateConnected {
		t.Fatalf("unexpected reconnected status: %+v", status)
	}
}

func TestReconnectDoesNotChangeStateWhenHostVerificationFails(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "connection-state")
	manager := NewClientConnectionManager(Config{}, statePath)
	if err := manager.Disconnect(); err != nil {
		t.Fatal(err)
	}
	manager.probe = func(context.Context, Config) error { return errors.New("host key verification failed") }
	if err := manager.Reconnect(context.Background()); err == nil {
		t.Fatal("expected reconnect verification to fail")
	}
	if status := manager.Status(); status.State != connectionStateDisconnected {
		t.Fatalf("failed verification changed connection state: %+v", status)
	}
}

func TestManagedCollectorSkipsRemoteInventoryWhileDisconnected(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "connection-state")
	manager := NewClientConnectionManager(Config{Name: "test-host", Host: "192.0.2.10", SSHUser: "developer", SSHPort: 2222}, statePath)
	if err := manager.Disconnect(); err != nil {
		t.Fatal(err)
	}
	collector := NewManagedCollector(fixedCollector{snapshot: Snapshot{Projects: []Project{{Name: "private", Path: "/private"}}}}, manager)
	snapshot := collector.Collect()
	if snapshot.Status != "disconnected" || len(snapshot.Projects) != 0 || snapshot.Host.Name != "test-host" {
		t.Fatalf("unexpected disconnected snapshot: %+v", snapshot)
	}
}

func TestManagedCollectorDoesNotIgnoreUnreadableConnectionState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "connection-state")
	if err := os.Mkdir(statePath, 0700); err != nil {
		t.Fatal(err)
	}
	manager := NewClientConnectionManager(Config{Name: "test-host"}, statePath)
	collector := NewManagedCollector(fixedCollector{snapshot: Snapshot{Status: "connected"}}, manager)
	snapshot := collector.Collect()
	if snapshot.Status != "unavailable" || snapshot.Connection.State != "unavailable" || snapshot.Message == "" {
		t.Fatalf("unreadable state was ignored: %+v", snapshot)
	}
}
