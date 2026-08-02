package dashboard

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	value := `src/$(touch /tmp/otherhost-dashboard-unsafe)`
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

func TestRemoteInventoryScriptDiscoversRepositoriesWithinHome(t *testing.T) {
	home := t.TempDir()
	repositories := []string{
		"root-repository",
		"work/nested-repository",
		"work/group/deep-repository",
		"custom/deep/root/preferred-repository",
	}
	ignored := []string{
		"work/group/too/deep-repository",
		".cache/hidden-repository",
		"work/node_modules/dependency-repository",
		"work/vendor/dependency-repository",
	}
	for _, repository := range append(repositories, ignored...) {
		if err := os.MkdirAll(filepath.Join(home, repository, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	command := exec.Command("bash", "-s")
	command.Env = append(os.Environ(), "HOME="+home)
	command.Stdin = strings.NewReader(remoteInventoryScript("custom/deep/root"))
	output, err := command.Output()
	if err != nil {
		t.Fatalf("remote inventory script failed: %v", err)
	}
	snapshot, err := parseRemoteReport(string(output))
	if err != nil {
		t.Fatalf("could not parse remote inventory: %v\n%s", err, output)
	}

	found := make(map[string]bool, len(snapshot.Projects))
	for _, project := range snapshot.Projects {
		found[project.Path] = true
	}
	for _, repository := range repositories {
		path := filepath.Join(home, repository)
		if !found[path] {
			t.Errorf("expected repository was not discovered: %s", path)
		}
	}
	for _, repository := range ignored {
		path := filepath.Join(home, repository)
		if found[path] {
			t.Errorf("ignored repository was discovered: %s", path)
		}
	}
}
