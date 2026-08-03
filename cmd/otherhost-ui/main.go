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

	var collector dashboard.Collector
	var projectLauncher dashboard.ProjectLauncher = dashboard.VSCodeLauncher{}
	var terminal dashboard.TerminalLauncher
	var deleter dashboard.ProjectDeleter
	sshAlias := "home-otherhost"
	if *demo {
		collector = demoCollector{}
		terminal = demoTerminalLauncher{}
	} else {
		if *configPath == "" {
			return errors.New("--config is required")
		}
		config, err := dashboard.LoadConfig(*configPath)
		if err != nil {
			return err
		}
		collector = dashboard.NewSSHCollector(config)
		projectLauncher = dashboard.NewVSCodeLauncher(config)
		terminal = dashboard.NewSSHTerminalLauncher(config)
		deleter = dashboard.NewSSHProjectDeleter(config)
		sshAlias = config.Name
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return dashboard.RunServerOnAddress(ctx, collector, projectLauncher, terminal, deleter, sshAlias, *listenAddress, *port, !*noOpen)
}

type demoCollector struct{}

func (demoCollector) Collect() dashboard.Snapshot {
	return dashboard.Snapshot{
		Status: "connected",
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
