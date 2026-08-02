package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
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
	var terminal dashboard.TerminalLauncher
	var deleter dashboard.ProjectDeleter
	sshAlias := "home-otherhost"
	if *demo {
		collector = demoCollector{}
	} else {
		if *configPath == "" {
			return errors.New("--config is required")
		}
		config, err := dashboard.LoadConfig(*configPath)
		if err != nil {
			return err
		}
		collector = dashboard.NewSSHCollector(config)
		terminal = dashboard.NewSSHTerminalLauncher(config)
		deleter = dashboard.NewSSHProjectDeleter(config)
		sshAlias = config.Name
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return dashboard.RunServer(ctx, collector, dashboard.VSCodeLauncher{}, terminal, deleter, sshAlias, *port, !*noOpen)
}

type demoCollector struct{}

func (demoCollector) Collect() dashboard.Snapshot {
	return dashboard.Snapshot{
		Status: "connected",
		Host: dashboard.Host{
			Name: "NEBULA-FORGE", OS: "Windows 11 Pro",
			CPU:    dashboard.CPU{Model: "Intel Core i7-14700KF", PhysicalCores: 20, LogicalProcessors: 28},
			Memory: 64 * 1024 * 1024 * 1024,
			GPU:    dashboard.GPU{Model: "NVIDIA GeForce RTX 5070", Memory: 12 * 1024 * 1024 * 1024},
			Disks: []dashboard.Disk{
				{Name: "C:", Total: 1_000_000_000_000, Available: 328_000_000_000},
				{Name: "F:", Total: 1_000_000_000_000, Available: 714_000_000_000},
			},
		},
		Environment: dashboard.Environment{
			Distribution: "Ubuntu 24.04 LTS", Kernel: "6.6.87.2-microsoft-standard-WSL2",
			Processors: 8, Memory: 20 * 1024 * 1024 * 1024, MemoryAvailable: 14 * 1024 * 1024 * 1024,
			Disk: dashboard.Disk{Name: "WSL", Total: 512 * 1024 * 1024 * 1024, Available: 391 * 1024 * 1024 * 1024},
		},
		Projects: []dashboard.Project{
			{Name: "aurora-api", Path: "/home/demo/src/aurora-api", Branch: "main", Technologies: []string{"Ruby", "Node.js"}},
			{Name: "otherhost", Path: "/home/demo/src/lumen-console", Branch: "feat/otherhost-rebrand", Technologies: []string{"Go"}},
			{Name: "vector-worker", Path: "/home/demo/src/vector-worker", Branch: "main", Technologies: []string{"Node.js"}},
		},
		SSHResponseMS: 18,
		UpdatedAt:     time.Now().UTC(),
	}
}
