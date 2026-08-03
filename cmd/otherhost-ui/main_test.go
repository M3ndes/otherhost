package main

import (
	"io"
	"strings"
	"testing"

	"github.com/M3ndes/otherhost/internal/dashboard"
)

func TestDemoCollectorUsesFictitiousPresentationData(t *testing.T) {
	snapshot := newDemoState("client").Collect()
	if snapshot.Host.Name != "NEBULA-FORGE" || snapshot.Host.Memory != 256*1024*1024*1024 {
		t.Fatalf("unexpected demonstration host: %+v", snapshot.Host)
	}
	if len(snapshot.Projects) != 3 {
		t.Fatalf("unexpected demonstration projects: %+v", snapshot.Projects)
	}
	for _, project := range snapshot.Projects {
		if !strings.HasPrefix(project.Path, "/home/demo/src/") {
			t.Fatalf("demonstration project uses a non-demo path: %s", project.Path)
		}
	}
}

func TestDemoTerminalProvidesScriptedFictitiousOutput(t *testing.T) {
	terminal, err := (demoTerminalLauncher{}).StartTerminal("/home/demo/src/aurora-api", dashboard.TerminalSize{Columns: 120, Rows: 32})
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()

	buffer := make([]byte, 4096)
	count, err := terminal.Read(buffer)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	output := string(buffer[:count])
	for _, expected := range []string{"\x1b[2J\x1b[H", "~/src/aurora-api", "128 tests passed", "docker compose ps"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("demonstration terminal is missing %q: %q", expected, output)
		}
	}
}
