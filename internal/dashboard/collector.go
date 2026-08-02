package dashboard

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

const collectionTimeout = 12 * time.Second

type SSHCollector struct {
	config Config
}

func NewSSHCollector(config Config) *SSHCollector {
	return &SSHCollector{config: config}
}

func (collector *SSHCollector) Collect() Snapshot {
	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), collectionTimeout)
	defer cancel()

	command := exec.CommandContext(ctx, "ssh", collector.sshArguments()...)
	command.Stdin = strings.NewReader(remoteInventoryScript(collector.config.ProjectsRoot))
	var standardOutput bytes.Buffer
	var standardError bytes.Buffer
	command.Stdout = &standardOutput
	command.Stderr = &standardError
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(standardError.String())
		if message == "" {
			if ctx.Err() != nil {
				message = "The remote host did not respond in time."
			} else {
				message = "The remote host is unavailable."
			}
		}
		return Snapshot{
			Status:    "unavailable",
			Message:   message,
			Host:      Host{Name: collector.config.Name},
			Projects:  []Project{},
			UpdatedAt: time.Now().UTC(),
		}
	}

	snapshot, err := parseRemoteReport(standardOutput.String())
	if err != nil {
		return Snapshot{
			Status:    "unavailable",
			Message:   "The host returned an unreadable inventory response.",
			Host:      Host{Name: collector.config.Name},
			Projects:  []Project{},
			UpdatedAt: time.Now().UTC(),
		}
	}
	if snapshot.Host.Name == "" {
		snapshot.Host.Name = collector.config.Name
	}
	snapshot.Status = "connected"
	snapshot.SSHResponseMS = time.Since(startedAt).Milliseconds()
	snapshot.UpdatedAt = time.Now().UTC()
	return snapshot
}

func (collector *SSHCollector) sshArguments() []string {
	arguments := sshConnectionArguments(collector.config)
	return append(arguments, sshTarget(collector.config), "bash -s")
}

func sshConnectionArguments(config Config) []string {
	arguments := []string{
		"-p", strconv.Itoa(config.SSHPort),
		"-i", config.IdentityFile,
		"-o", "IdentitiesOnly=yes",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "ConnectTimeout=5",
		"-o", "ServerAliveInterval=5",
		"-o", "ServerAliveCountMax=1",
	}
	if config.KnownHostsFile != "" {
		arguments = append(arguments,
			"-o", "UserKnownHostsFile="+config.KnownHostsFile,
		)
	}
	return arguments
}

func sshTarget(config Config) string {
	return config.SSHUser + "@" + config.Host
}

type windowsInventory struct {
	OSName            string `json:"osName"`
	CPUModel          string `json:"cpuModel"`
	PhysicalCores     int    `json:"physicalCores"`
	LogicalProcessors int    `json:"logicalProcessors"`
	MemoryBytes       uint64 `json:"memoryBytes"`
	GPUModel          string `json:"gpuModel"`
	GPUMemoryBytes    uint64 `json:"gpuMemoryBytes"`
	Disks             []Disk `json:"disks"`
}

func parseRemoteReport(report string) (Snapshot, error) {
	snapshot := Snapshot{Projects: []Project{}, Host: Host{Disks: []Disk{}}}
	var currentProject *Project
	for _, line := range strings.Split(strings.Trim(report, "\r\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			return Snapshot{}, fmt.Errorf("invalid inventory line")
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(parts[1]))
		if err != nil {
			return Snapshot{}, fmt.Errorf("invalid inventory value: %w", err)
		}
		value := string(decoded)
		switch parts[0] {
		case "host.name":
			snapshot.Host.Name = value
		case "host.windows":
			var inventory windowsInventory
			value = strings.TrimPrefix(value, "\ufeff")
			if err := json.Unmarshal([]byte(value), &inventory); err != nil {
				continue
			}
			snapshot.Host.OS = strings.TrimSpace(inventory.OSName)
			snapshot.Host.CPU = CPU{
				Model: strings.TrimSpace(inventory.CPUModel), PhysicalCores: inventory.PhysicalCores,
				LogicalProcessors: inventory.LogicalProcessors,
			}
			snapshot.Host.Memory = inventory.MemoryBytes
			snapshot.Host.GPU = GPU{Model: strings.TrimSpace(inventory.GPUModel), Memory: inventory.GPUMemoryBytes}
			snapshot.Host.Disks = inventory.Disks
		case "environment.distribution":
			snapshot.Environment.Distribution = value
		case "environment.kernel":
			snapshot.Environment.Kernel = value
		case "environment.processors":
			snapshot.Environment.Processors, _ = strconv.Atoi(value)
		case "environment.memory":
			snapshot.Environment.Memory, _ = strconv.ParseUint(value, 10, 64)
		case "environment.memory_available":
			snapshot.Environment.MemoryAvailable, _ = strconv.ParseUint(value, 10, 64)
		case "environment.disk_total":
			snapshot.Environment.Disk.Name = "WSL"
			snapshot.Environment.Disk.Total, _ = strconv.ParseUint(value, 10, 64)
		case "environment.disk_available":
			snapshot.Environment.Disk.Available, _ = strconv.ParseUint(value, 10, 64)
		case "project.path":
			projectPath := value
			project := Project{Name: path.Base(projectPath), Path: projectPath, Technologies: []string{}}
			snapshot.Projects = append(snapshot.Projects, project)
			currentProject = &snapshot.Projects[len(snapshot.Projects)-1]
		case "project.branch":
			if currentProject != nil {
				currentProject.Branch = value
			}
		case "project.technology":
			if currentProject != nil && value != "" {
				currentProject.Technologies = append(currentProject.Technologies, value)
			}
		}
	}
	if snapshot.Host.CPU.Model == "" {
		snapshot.Host.CPU.Model = "Remote processor"
	}
	sort.Slice(snapshot.Projects, func(left, right int) bool {
		return strings.ToLower(snapshot.Projects[left].Name) < strings.ToLower(snapshot.Projects[right].Name)
	})
	return snapshot, nil
}

func remoteInventoryScript(projectsRoot string) string {
	projectsRootValue := base64.StdEncoding.EncodeToString([]byte(projectsRoot))
	powerShellValue := encodePowerShell(windowsInventoryScript)
	return strings.ReplaceAll(strings.ReplaceAll(remoteScriptTemplate,
		"__PROJECTS_ROOT__", projectsRootValue), "__POWERSHELL__", powerShellValue)
}

func encodePowerShell(script string) string {
	encodedRunes := utf16.Encode([]rune(script))
	encodedBytes := make([]byte, len(encodedRunes)*2)
	for index, value := range encodedRunes {
		binary.LittleEndian.PutUint16(encodedBytes[index*2:], value)
	}
	return base64.StdEncoding.EncodeToString(encodedBytes)
}

const windowsInventoryScript = `$ErrorActionPreference = "Stop"
$computer = Get-CimInstance Win32_ComputerSystem
$processor = Get-CimInstance Win32_Processor | Select-Object -First 1
$operatingSystem = Get-CimInstance Win32_OperatingSystem
$gpuModel = ""
$gpuMemoryBytes = 0
$nvidia = Get-Command nvidia-smi.exe -ErrorAction SilentlyContinue
if ($null -ne $nvidia) {
  $gpuLine = & $nvidia.Source --query-gpu=name,memory.total --format=csv,noheader,nounits 2>$null | Select-Object -First 1
  if ($gpuLine) {
    $gpuParts = $gpuLine -split ",", 2
    $gpuModel = $gpuParts[0].Trim()
    if ($gpuParts.Count -gt 1) { $gpuMemoryBytes = [int64]$gpuParts[1].Trim() * 1MB }
  }
}
if (-not $gpuModel) {
  $adapter = Get-CimInstance Win32_VideoController | Where-Object { $_.Name -notmatch "Microsoft" } | Select-Object -First 1
  if ($null -ne $adapter) {
    $gpuModel = $adapter.Name
    if ($adapter.AdapterRAM) { $gpuMemoryBytes = [int64]$adapter.AdapterRAM }
  }
}
$disks = @(Get-CimInstance Win32_LogicalDisk -Filter "DriveType=3" | Sort-Object DeviceID | ForEach-Object {
  [ordered]@{ name = $_.DeviceID; totalBytes = [int64]$_.Size; availableBytes = [int64]$_.FreeSpace }
})
[ordered]@{
  osName = $operatingSystem.Caption
  cpuModel = $processor.Name.Trim()
  physicalCores = [int]$processor.NumberOfCores
  logicalProcessors = [int]$processor.NumberOfLogicalProcessors
  memoryBytes = [int64]$computer.TotalPhysicalMemory
  gpuModel = $gpuModel
  gpuMemoryBytes = $gpuMemoryBytes
  disks = $disks
} | ConvertTo-Json -Compress -Depth 4`

const remoteScriptTemplate = `set -u

emit() {
  key=$1
  value=$2
  encoded=$(printf '%s' "$value" | base64 | tr -d '\r\n')
  printf '%s\t%s\n' "$key" "$encoded"
}

emit host.name "$(hostname)"

distribution='Linux'
if [ -r /etc/os-release ]; then
  distribution=$(awk -F= '$1 == "PRETTY_NAME" { value=$2; gsub(/^"|"$/, "", value); print value; exit }' /etc/os-release)
fi
emit environment.distribution "$distribution"
emit environment.kernel "$(uname -r)"
emit environment.processors "$(getconf _NPROCESSORS_ONLN 2>/dev/null || nproc)"
emit environment.memory "$(awk '/^MemTotal:/ { print $2 * 1024; exit }' /proc/meminfo)"
emit environment.memory_available "$(awk '/^MemAvailable:/ { print $2 * 1024; exit }' /proc/meminfo)"
emit environment.disk_total "$(df -B1 --output=size "$HOME" | awk 'NR == 2 { print $1 }')"
emit environment.disk_available "$(df -B1 --output=avail "$HOME" | awk 'NR == 2 { print $1 }')"

power_shell=''
if command -v powershell.exe >/dev/null 2>&1; then
  power_shell=$(command -v powershell.exe)
elif [ -x /mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe ]; then
  power_shell=/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe
fi
if [ -n "$power_shell" ]; then
  windows_json=$("$power_shell" -NoLogo -NoProfile -NonInteractive -EncodedCommand __POWERSHELL__ </dev/null 2>/dev/null | tr -d '\r' || true)
  if [ -n "$windows_json" ]; then emit host.windows "$windows_json"; fi
fi

projects_relative=$(printf '%s' '__PROJECTS_ROOT__' | base64 -d)
projects_root="$HOME/$projects_relative"

discover_project_candidates() {
  search_root=$1
  search_depth=$2
  [ -d "$search_root" ] || return 0
  find "$search_root" -mindepth 1 -maxdepth "$search_depth" \
    \( -type d \( -name '.*' -o -name node_modules -o -name vendor \) -prune \) -o \
    -type d -print0
}

{
  discover_project_candidates "$HOME" 3
  discover_project_candidates "$projects_root" 1
} | sort -zu | {
  project_count=0
  while IFS= read -r -d '' project; do
    # Standard submodules and linked worktrees use a .git pointer file. Only
    # primary repository checkouts own a .git directory and belong in Projects.
    if [ ! -d "$project/.git" ]; then continue; fi
    emit project.path "$project"
    branch=$(git -C "$project" branch --show-current 2>/dev/null || true)
    emit project.branch "$branch"
    [ -f "$project/Gemfile" ] && emit project.technology 'Ruby'
    [ -f "$project/package.json" ] && emit project.technology 'Node.js'
    [ -f "$project/go.mod" ] && emit project.technology 'Go'
    [ -f "$project/Cargo.toml" ] && emit project.technology 'Rust'
    if [ -f "$project/pyproject.toml" ] || [ -f "$project/requirements.txt" ]; then emit project.technology 'Python'; fi
    project_count=$((project_count + 1))
    if [ "$project_count" -ge 200 ]; then break; fi
  done
}
`
