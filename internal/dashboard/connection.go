package dashboard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	connectionStateConnected    = "connected"
	connectionStateDisconnected = "disconnected"
)

type ConnectionManager interface {
	Status() Connection
	Disconnect() error
	Reconnect(context.Context) error
}

type ClientConnectionManager struct {
	config    Config
	statePath string
	probe     func(context.Context, Config) error
}

func NewClientConnectionManager(config Config, statePath string) *ClientConnectionManager {
	if statePath == "" {
		statePath = DefaultConnectionStatePath()
	}
	return &ClientConnectionManager{config: config, statePath: statePath, probe: probeSSHConnection}
}

func DefaultConnectionStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", "otherhost", "connection-state")
}

func (manager *ClientConnectionManager) Status() Connection {
	state := connectionStateConnected
	if content, err := os.ReadFile(manager.statePath); err == nil {
		if strings.TrimSpace(string(content)) == connectionStateDisconnected {
			state = connectionStateDisconnected
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return manager.connection("unavailable", fmt.Sprintf("Could not read the local connection state: %v", err))
	}
	return manager.connection(state, "")
}

func (manager *ClientConnectionManager) connection(state, message string) Connection {
	return Connection{
		State:          state,
		Paired:         manager.config.Host != "" && manager.config.SSHUser != "" && manager.config.SSHPort > 0,
		HostName:       manager.config.Name,
		HostAddress:    manager.config.Host,
		SSHUser:        manager.config.SSHUser,
		SSHPort:        manager.config.SSHPort,
		IdentityPinned: manager.config.KnownHostsFile != "",
		Message:        message,
	}
}

func (manager *ClientConnectionManager) Disconnect() error {
	return writeConnectionState(manager.statePath, connectionStateDisconnected)
}

func (manager *ClientConnectionManager) Reconnect(ctx context.Context) error {
	if err := manager.probe(ctx, manager.config); err != nil {
		return err
	}
	return writeConnectionState(manager.statePath, connectionStateConnected)
}

func writeConnectionState(path, state string) error {
	if path == "" {
		return errors.New("connection state path is unavailable")
	}
	if state != connectionStateConnected && state != connectionStateDisconnected {
		return errors.New("invalid connection state")
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return fmt.Errorf("could not create the connection state directory: %w", err)
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return fmt.Errorf("could not protect the connection state directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".connection-state-*")
	if err != nil {
		return fmt.Errorf("could not create the connection state: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.WriteString(state + "\n"); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("could not save the connection state: %w", err)
	}
	return nil
}

func probeSSHConnection(ctx context.Context, config Config) error {
	probeContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	arguments := append(sshConnectionArguments(config), sshTarget(config), "exit 0")
	output, err := exec.CommandContext(probeContext, "ssh", arguments...).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = "The saved host is unavailable."
		}
		return errors.New(message)
	}
	return nil
}

type ManagedCollector struct {
	collector  Collector
	connection ConnectionManager
}

func NewManagedCollector(collector Collector, connection ConnectionManager) *ManagedCollector {
	return &ManagedCollector{collector: collector, connection: connection}
}

func (collector *ManagedCollector) Collect() Snapshot {
	connection := collector.connection.Status()
	if connection.State == connectionStateDisconnected {
		return Snapshot{
			Mode:       "client",
			Status:     "disconnected",
			Message:    "Connection paused on this Mac.",
			Host:       Host{Name: connection.HostName},
			Projects:   []Project{},
			Clients:    []Client{},
			Sessions:   []Session{},
			Connection: connection,
			UpdatedAt:  time.Now().UTC(),
		}
	}
	if connection.State != connectionStateConnected {
		return Snapshot{
			Mode:       "client",
			Status:     "unavailable",
			Message:    connection.Message,
			Host:       Host{Name: connection.HostName},
			Projects:   []Project{},
			Clients:    []Client{},
			Sessions:   []Session{},
			Connection: connection,
			UpdatedAt:  time.Now().UTC(),
		}
	}
	snapshot := collector.collector.Collect()
	snapshot.Mode = "client"
	snapshot.Connection = connection
	if snapshot.Clients == nil {
		snapshot.Clients = []Client{}
	}
	if snapshot.Sessions == nil {
		snapshot.Sessions = []Session{}
	}
	return snapshot
}
