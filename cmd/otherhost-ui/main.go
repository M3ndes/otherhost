package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/M3ndes/otherhost/internal/dashboard"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "[fail] %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	flags := flag.NewFlagSet("otherhost-ui", flag.ContinueOnError)
	configPath := flags.String("config", "", "otherhost configuration path")
	mode := flags.String("mode", "client", "dashboard mode: client or host")
	repository := flags.String("repository", "", "path to the Otherhost repository (host mode)")
	distribution := flags.String("distribution", "Ubuntu", "WSL distribution used by the Windows host")
	connectionState := flags.String("connection-state", "", "client connection state path")
	listenAddress := flags.String("listen-address", "127.0.0.1", "dashboard listen address")
	port := flags.Int("port", 7842, "local loopback port")
	noOpen := flags.Bool("no-open", false, "do not open the browser automatically")
	demo := flags.Bool("demo", false, "use local demonstration data")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("otherhost-ui does not accept positional arguments")
	}
	if *mode != "client" && *mode != "host" {
		return errors.New("--mode must be client or host")
	}

	var collector dashboard.Collector
	var projectLauncher dashboard.ProjectLauncher = dashboard.VSCodeLauncher{}
	var terminal dashboard.TerminalLauncher
	var deleter dashboard.ProjectDeleter
	var connection dashboard.ConnectionManager
	var host dashboard.HostController
	sshAlias := "home-otherhost"
	if *demo {
		demo := newDemoState(*mode)
		collector = demo
		if *mode == "client" {
			connection = demo
			terminal = demoTerminalLauncher{}
		} else {
			host = demo
		}
	} else if *mode == "host" {
		if runtime.GOOS != "windows" {
			return errors.New("host mode must run on Windows; use --demo to preview it elsewhere")
		}
		if *repository == "" {
			workingDirectory, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("could not determine the Otherhost repository: %w", err)
			}
			*repository = workingDirectory
		}
		collector = dashboard.NewWindowsHostCollector(*distribution)
		host = dashboard.NewWindowsHostController(*repository, *distribution)
		sshAlias = ""
	} else {
		if *configPath == "" {
			return errors.New("--config is required")
		}
		config, err := dashboard.LoadConfig(*configPath)
		if err != nil {
			return err
		}
		connection = dashboard.NewClientConnectionManager(config, *connectionState)
		collector = dashboard.NewManagedCollector(dashboard.NewSSHCollector(config), connection)
		projectLauncher = dashboard.NewVSCodeLauncher(config)
		terminal = dashboard.NewSSHTerminalLauncher(config)
		deleter = dashboard.NewSSHProjectDeleter(config)
		sshAlias = config.Name
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return dashboard.RunManagedServerOnAddress(ctx, collector, projectLauncher, terminal, deleter, connection, host, sshAlias, *listenAddress, *port, !*noOpen)
}

type demoState struct {
	mutex     sync.Mutex
	mode      string
	connected bool
	clients   []dashboard.Client
	message   string
}

func newDemoState(mode string) *demoState {
	return &demoState{
		mode: mode, connected: true,
		clients: []dashboard.Client{
			{Name: "Orion MacBook", Fingerprint: "SHA256:Jqf5bXw16tjmDBpJaL9OyQpK0mZL3RHHFBnM0PqgwQ4", Authorized: true},
			{Name: "Studio Mac", Fingerprint: "SHA256:F7fR0V3NHz4IbJ0DqfX0AEeHdWodNpxlG0n67jF8PqI", Authorized: true},
		},
	}
}

func (demo *demoState) Collect() dashboard.Snapshot {
	demo.mutex.Lock()
	defer demo.mutex.Unlock()
	status := "connected"
	if demo.mode == "client" && !demo.connected {
		status = "disconnected"
	}
	snapshot := dashboard.Snapshot{
		Mode: demo.mode, Status: status,
		Host: dashboard.Host{
			Name: "NEBULA-FORGE", OS: "Windows 11 Pro for Workstations",
			CPU:    dashboard.CPU{Model: "AMD TR PRO 7995WX", PhysicalCores: 96, LogicalProcessors: 192},
			Memory: 256 * 1024 * 1024 * 1024,
			GPU:    dashboard.GPU{Model: "NVIDIA RTX 6000 Ada", Memory: 48 * 1024 * 1024 * 1024},
			Disks: []dashboard.Disk{
				{Name: "C:", Total: 4 * 1024 * 1024 * 1024 * 1024, Available: 3 * 1024 * 1024 * 1024 * 1024},
				{Name: "D:", Total: 8 * 1024 * 1024 * 1024 * 1024, Available: 6 * 1024 * 1024 * 1024 * 1024},
			},
		},
		Environment: dashboard.Environment{
			Distribution: "Ubuntu 24.04 LTS", Kernel: "6.6.87.2-microsoft-standard-WSL2",
			Processors: 64, Memory: 128 * 1024 * 1024 * 1024, MemoryAvailable: 104 * 1024 * 1024 * 1024,
			Disk: dashboard.Disk{Name: "WSL", Total: 4 * 1024 * 1024 * 1024 * 1024, Available: 3 * 1024 * 1024 * 1024 * 1024},
		},
		Projects: []dashboard.Project{
			{Name: "aurora-api", Path: "/home/demo/src/aurora-api", Branch: "main", Technologies: []string{"Go"}},
			{Name: "lumen-console", Path: "/home/demo/src/lumen-console", Branch: "feat/command-palette", Technologies: []string{"Node.js"}},
			{Name: "vector-worker", Path: "/home/demo/src/vector-worker", Branch: "experiment/gpu-jobs", Technologies: []string{"Python", "Rust"}},
		},
		SSHResponseMS: 7,
		UpdatedAt:     time.Now().UTC(),
	}
	if demo.mode == "client" {
		snapshot.Connection = dashboard.Connection{
			State: status, Paired: true, HostName: snapshot.Host.Name, HostAddress: "192.0.2.40",
			SSHUser: "developer", SSHPort: 2222, IdentityPinned: true,
		}
		if !demo.connected {
			snapshot.Message = "Connection paused on this Mac."
			snapshot.Projects = []dashboard.Project{}
		}
		return snapshot
	}
	snapshot.Status = "ready"
	snapshot.Connection = dashboard.Connection{State: "host"}
	snapshot.Projects = []dashboard.Project{}
	snapshot.Setup = dashboard.HostSetup{
		State: "ready", Message: demo.message,
		Steps: []dashboard.SetupStep{
			{ID: "windows", Label: "Supported Windows host", Status: "ready", Message: "Windows 11 Pro for Workstations"},
			{ID: "wsl", Label: "WSL 2 and Ubuntu", Status: "ready", Message: "Ubuntu 24.04 LTS"},
			{ID: "otherhost", Label: "Otherhost host policy", Status: "ready", Message: "Configuration current"},
			{ID: "ssh", Label: "Hardened SSH service", Status: "ready", Message: "Public-key access"},
			{ID: "docker", Label: "Docker Desktop integration", Status: "ready", Message: "Docker Engine running"},
		},
	}
	snapshot.Clients = append([]dashboard.Client(nil), demo.clients...)
	snapshot.Sessions = []dashboard.Session{{Address: "192.0.2.12:53184", State: "active"}}
	return snapshot
}

func (demo *demoState) Status() dashboard.Connection {
	return demo.Collect().Connection
}

func (demo *demoState) Disconnect() error {
	demo.mutex.Lock()
	demo.connected = false
	demo.mutex.Unlock()
	return nil
}

func (demo *demoState) Reconnect(context.Context) error {
	demo.mutex.Lock()
	demo.connected = true
	demo.mutex.Unlock()
	return nil
}

func (demo *demoState) ActionState() (bool, string) {
	demo.mutex.Lock()
	defer demo.mutex.Unlock()
	return false, demo.message
}

func (demo *demoState) Configure() error {
	demo.mutex.Lock()
	demo.message = "Host configuration is complete."
	demo.mutex.Unlock()
	return nil
}

func (demo *demoState) EnablePairing() error {
	demo.mutex.Lock()
	demo.message = "Pairing discovery is available for two minutes."
	demo.mutex.Unlock()
	return nil
}

func (demo *demoState) Revoke(_ context.Context, fingerprint string) error {
	demo.mutex.Lock()
	defer demo.mutex.Unlock()
	for index, client := range demo.clients {
		if client.Fingerprint == fingerprint {
			demo.clients = append(demo.clients[:index], demo.clients[index+1:]...)
			return nil
		}
	}
	return errors.New("selected client is no longer authorized")
}

type demoTerminalLauncher struct{}

func (demoTerminalLauncher) StartTerminal(projectPath string, _ dashboard.TerminalSize) (dashboard.Terminal, error) {
	location := "~"
	if projectPath != "" {
		location = "~/src/" + path.Base(projectPath)
	}
	return newDemoTerminal(location), nil
}

type demoTerminal struct {
	reader    *io.PipeReader
	writer    *io.PipeWriter
	closeOnce sync.Once
}

func newDemoTerminal(location string) *demoTerminal {
	reader, writer := io.Pipe()
	terminal := &demoTerminal{reader: reader, writer: writer}
	go func() {
		_, _ = io.WriteString(writer, demoTerminalOutput(location))
	}()
	return terminal
}

func demoTerminalOutput(location string) string {
	prompt := "\x1b[38;5;141m" + location + "\x1b[0m \x1b[1m❯\x1b[0m "
	return "\x1b[2J\x1b[H" +
		prompt + "make test\r\n" +
		"\x1b[32m✓\x1b[0m 128 tests passed in 4.2s\r\n\r\n" +
		prompt + "docker compose ps\r\n" +
		"NAME             STATUS          PORTS\r\n" +
		"aurora-api       Up 2 minutes    0.0.0.0:8080->8080/tcp\r\n" +
		"postgres         Up 2 minutes    5432/tcp\r\n" +
		"redis            Up 2 minutes    6379/tcp\r\n\r\n" +
		prompt
}

func (terminal *demoTerminal) Read(buffer []byte) (int, error) {
	return terminal.reader.Read(buffer)
}

func (terminal *demoTerminal) Write(buffer []byte) (int, error) {
	return len(buffer), nil
}

func (terminal *demoTerminal) Resize(dashboard.TerminalSize) error {
	return nil
}

func (terminal *demoTerminal) Close() error {
	var closeError error
	terminal.closeOnce.Do(func() {
		closeError = errors.Join(terminal.writer.Close(), terminal.reader.Close())
	})
	return closeError
}
