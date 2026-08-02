package dashboard

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func reportLine(key, value string) string {
	return fmt.Sprintf("%s\t%s", key, base64.StdEncoding.EncodeToString([]byte(value)))
}

func TestParseRemoteReportBuildsMachineAndProjectInventory(t *testing.T) {
	windows := `{"osName":"Windows 11 Pro","cpuModel":"Intel Core i7-14700KF","physicalCores":20,"logicalProcessors":28,"memoryBytes":68719476736,"gpuModel":"NVIDIA GeForce RTX 5070","gpuMemoryBytes":12884901888,"disks":[{"name":"C:","totalBytes":1000000000000,"availableBytes":328000000000}]}`
	report := strings.Join([]string{
		reportLine("host.name", "DESKTOP-HOME"),
		reportLine("host.windows", windows),
		reportLine("environment.distribution", "Ubuntu 24.04 LTS"),
		reportLine("environment.processors", "8"),
		reportLine("environment.memory", "21474836480"),
		reportLine("project.path", "/home/developer/src/zeta"),
		reportLine("project.branch", "main"),
		reportLine("project.technology", "Go"),
		reportLine("project.path", "/home/developer/src/alpha"),
		reportLine("project.branch", "feat/dashboard"),
		reportLine("project.technology", "Ruby"),
	}, "\n")

	snapshot, err := parseRemoteReport(report)
	if err != nil {
		t.Fatalf("parseRemoteReport returned an error: %v", err)
	}
	if snapshot.Host.CPU.PhysicalCores != 20 || snapshot.Host.CPU.LogicalProcessors != 28 {
		t.Fatalf("unexpected CPU inventory: %#v", snapshot.Host.CPU)
	}
	if snapshot.Host.GPU.Memory != 12*1024*1024*1024 {
		t.Fatalf("unexpected GPU memory: %d", snapshot.Host.GPU.Memory)
	}
	if len(snapshot.Projects) != 2 || snapshot.Projects[0].Name != "alpha" || snapshot.Projects[1].Name != "zeta" {
		t.Fatalf("projects were not sorted: %#v", snapshot.Projects)
	}
	if snapshot.Projects[0].Branch != "feat/dashboard" || snapshot.Projects[0].Technologies[0] != "Ruby" {
		t.Fatalf("project metadata was not associated correctly: %#v", snapshot.Projects[0])
	}
}

func TestRemoteInventoryScriptDoesNotInterpolateProjectPath(t *testing.T) {
	value := `src/$(touch /tmp/devbox-dashboard-unsafe)`
	script := remoteInventoryScript(value)
	if strings.Contains(script, value) {
		t.Fatal("projects root was interpolated into the remote shell script")
	}
	if !strings.Contains(script, base64.StdEncoding.EncodeToString([]byte(value))) {
		t.Fatal("encoded projects root is missing")
	}
	if !strings.Contains(script, "/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe") {
		t.Fatal("restricted WSL SSH sessions cannot find the absolute PowerShell fallback")
	}
	if !strings.Contains(script, "</dev/null") {
		t.Fatal("Windows inventory command can consume the remaining streamed shell script")
	}
}
