package dashboard

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/M3ndes/otherhost/internal/pairing"
)

type commandRunner interface {
	Run(context.Context, string, []string, string) ([]byte, error)
	Start(string, []string) (<-chan error, error)
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, name string, arguments []string, standardInput string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Stdin = strings.NewReader(standardInput)
	return command.CombinedOutput()
}

func (execCommandRunner) Start(name string, arguments []string) (<-chan error, error) {
	command := exec.Command(name, arguments...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
		close(done)
	}()
	return done, nil
}

type WindowsHostCollector struct {
	distribution string
	runner       commandRunner
}

func NewWindowsHostCollector(distribution string) *WindowsHostCollector {
	return &WindowsHostCollector{distribution: distribution, runner: execCommandRunner{}}
}

func (collector *WindowsHostCollector) Collect() Snapshot {
	snapshot := Snapshot{
		Mode:       "host",
		Status:     "unavailable",
		Host:       Host{Disks: []Disk{}},
		Projects:   []Project{},
		Clients:    []Client{},
		Sessions:   []Session{},
		Setup:      defaultHostSetup(),
		UpdatedAt:  time.Now().UTC(),
		Connection: Connection{State: "host"},
	}
	if runtime.GOOS != "windows" {
		snapshot.Message = "Host mode is available only on Windows."
		return snapshot
	}

	ctx, cancel := context.WithTimeout(context.Background(), collectionTimeout)
	defer cancel()
	powerShellOutput, err := collector.runner.Run(ctx, "powershell.exe", []string{
		"-NoProfile", "-NonInteractive", "-EncodedCommand", encodePowerShell(windowsInventoryScript),
	}, "")
	if err != nil {
		snapshot.Message = "Could not inspect this Windows host."
		return snapshot
	}
	var inventory windowsInventory
	if err := json.Unmarshal(bytes.TrimPrefix(bytes.TrimSpace(powerShellOutput), []byte("\xef\xbb\xbf")), &inventory); err != nil {
		snapshot.Message = "Windows returned an unreadable host inventory."
		return snapshot
	}
	snapshot.Host = Host{
		Name: os.Getenv("COMPUTERNAME"), OS: strings.TrimSpace(inventory.OSName),
		CPU:    CPU{Model: strings.TrimSpace(inventory.CPUModel), PhysicalCores: inventory.PhysicalCores, LogicalProcessors: inventory.LogicalProcessors},
		Memory: inventory.MemoryBytes, GPU: GPU{Model: strings.TrimSpace(inventory.GPUModel), Memory: inventory.GPUMemoryBytes},
		Disks: inventory.Disks,
	}
	if snapshot.Host.Name == "" {
		snapshot.Host.Name = "Windows host"
	}
	snapshot.Setup.Steps[0].Status = "ready"
	snapshot.Setup.Steps[0].Message = snapshot.Host.OS

	wslOutput, wslErr := collector.runner.Run(ctx, "wsl.exe", []string{"-d", collector.distribution, "--", "bash", "-s"}, hostInventoryScript())
	if wslErr != nil {
		snapshot.Message = "WSL is not ready. Complete host setup to continue."
		snapshot.Setup.Message = snapshot.Message
		return snapshot
	}
	parseHostReport(string(wslOutput), &snapshot)
	snapshot.Status = "ready"
	if snapshot.Setup.State != "ready" {
		snapshot.Status = "needs_setup"
		snapshot.Message = snapshot.Setup.Message
	}
	return snapshot
}

func defaultHostSetup() HostSetup {
	return HostSetup{
		State: "needs_setup",
		Steps: []SetupStep{
			{ID: "windows", Label: "Supported Windows host", Status: "checking"},
			{ID: "wsl", Label: "WSL 2 and Ubuntu", Status: "checking"},
			{ID: "otherhost", Label: "Otherhost host policy", Status: "checking"},
			{ID: "ssh", Label: "Hardened SSH service", Status: "checking"},
			{ID: "docker", Label: "Docker Desktop integration", Status: "checking"},
		},
	}
}

func hostInventoryScript() string {
	return `set -u
encode() { printf '%s ' "$1"; printf '%s' "$2" | base64 | tr -d '\n'; printf '\n'; }
state_file="$HOME/.local/state/otherhost/install-state"
if [ -f "$state_file" ]; then encode setup.otherhost ready; else encode setup.otherhost missing; fi
if command -v systemctl >/dev/null 2>&1 && { systemctl is-active --quiet ssh || systemctl --user is-active --quiet otherhost-sshd.service || systemctl --user is-active --quiet devbox-bridge-sshd.service; }; then encode setup.ssh ready; else encode setup.ssh missing; fi
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then encode setup.docker ready; else encode setup.docker optional; fi
encode environment.distribution "$(. /etc/os-release 2>/dev/null && printf '%s' "${PRETTY_NAME:-WSL}" || printf WSL)"
encode environment.kernel "$(uname -r 2>/dev/null || true)"
encode environment.processors "$(getconf _NPROCESSORS_ONLN 2>/dev/null || nproc 2>/dev/null || true)"
encode environment.memory "$(awk '/^MemTotal:/ { print $2 * 1024 }' /proc/meminfo 2>/dev/null)"
encode environment.memory_available "$(awk '/^MemAvailable:/ { print $2 * 1024 }' /proc/meminfo 2>/dev/null)"
encode environment.disk_total "$(df -B1 --output=size "$HOME" 2>/dev/null | awk 'NR == 2 { print $1 }')"
encode environment.disk_available "$(df -B1 --output=avail "$HOME" 2>/dev/null | awk 'NR == 2 { print $1 }')"
if [ -f "$HOME/.ssh/authorized_keys" ]; then
  while IFS= read -r key; do
    case "$key" in ''|'#'*) continue ;; esac
    encode client.key "$key"
  done < "$HOME/.ssh/authorized_keys"
fi
if command -v ss >/dev/null 2>&1; then
  ssh_port=$(awk -F= '$1 ~ /^[[:space:]]*ssh_port[[:space:]]*$/ { gsub(/[[:space:]]/, "", $2); print $2; exit }' "$HOME/src/otherhost/otherhost.local.conf" 2>/dev/null || true)
  case "$ssh_port" in ''|*[!0-9]*) ssh_port=2222 ;; esac
  ss -Htn state established 2>/dev/null | awk -v suffix=":$ssh_port" 'index($4, suffix) == length($4) - length(suffix) + 1 { print $5 }' | while IFS= read -r peer; do
    [ -n "$peer" ] && encode session.address "$peer"
  done
fi
`
}

func parseHostReport(report string, snapshot *Snapshot) {
	setupValues := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(report), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), " ", 2)
		if len(parts) != 2 {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			continue
		}
		value := string(decoded)
		switch parts[0] {
		case "setup.otherhost", "setup.ssh", "setup.docker":
			setupValues[parts[0]] = value
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
			snapshot.Environment.Disk = Disk{Name: "WSL"}
			snapshot.Environment.Disk.Total, _ = strconv.ParseUint(value, 10, 64)
		case "environment.disk_available":
			snapshot.Environment.Disk.Available, _ = strconv.ParseUint(value, 10, 64)
		case "client.key":
			fingerprint, err := pairing.SSHPublicKeyFingerprint(value)
			if err == nil {
				snapshot.Clients = append(snapshot.Clients, Client{Fingerprint: fingerprint, Name: publicKeyComment(value), Authorized: true})
			}
		case "session.address":
			snapshot.Sessions = append(snapshot.Sessions, Session{Address: value, State: "active"})
		}
	}
	setSetupStep(snapshot.Setup.Steps, "wsl", "ready", snapshot.Environment.Distribution)
	setSetupStep(snapshot.Setup.Steps, "otherhost", setupStatus(setupValues["setup.otherhost"]), "Revision state and policy")
	setSetupStep(snapshot.Setup.Steps, "ssh", setupStatus(setupValues["setup.ssh"]), "Public-key access")
	dockerStatus := setupStatus(setupValues["setup.docker"])
	if setupValues["setup.docker"] == "optional" {
		dockerStatus = "optional"
	}
	setSetupStep(snapshot.Setup.Steps, "docker", dockerStatus, "Optional for SSH; recommended for workloads")
	if setupValues["setup.otherhost"] == "ready" && setupValues["setup.ssh"] == "ready" {
		snapshot.Setup.State = "ready"
		snapshot.Setup.Message = "This machine is ready to accept a paired Mac."
	} else {
		snapshot.Setup.State = "needs_setup"
		snapshot.Setup.Message = "Complete the setup wizard before enabling pairing."
	}
}

func setupStatus(value string) string {
	if value == "ready" {
		return "ready"
	}
	return "action_required"
}

func setSetupStep(steps []SetupStep, id, status, message string) {
	for index := range steps {
		if steps[index].ID == id {
			steps[index].Status = status
			steps[index].Message = message
			return
		}
	}
}

func publicKeyComment(publicKey string) string {
	fields := strings.Fields(publicKey)
	if len(fields) < 3 {
		return "Authorized Mac"
	}
	return strings.Join(fields[2:], " ")
}

type HostController interface {
	ActionState() (bool, string)
	Configure() error
	EnablePairing() error
	Revoke(context.Context, string) error
}

type WindowsHostController struct {
	repository   string
	distribution string
	runner       commandRunner

	mutex   sync.RWMutex
	busy    bool
	message string
}

func NewWindowsHostController(repository, distribution string) *WindowsHostController {
	return &WindowsHostController{repository: repository, distribution: distribution, runner: execCommandRunner{}}
}

func (controller *WindowsHostController) ActionState() (bool, string) {
	controller.mutex.RLock()
	defer controller.mutex.RUnlock()
	return controller.busy, controller.message
}

func (controller *WindowsHostController) start(arguments ...string) error {
	if runtime.GOOS != "windows" {
		return errors.New("host actions are available only on Windows")
	}
	controller.mutex.Lock()
	if controller.busy {
		controller.mutex.Unlock()
		return errors.New("another host action is already running")
	}
	controller.busy = true
	controller.message = "Follow the PowerShell window and approve Windows elevation when requested."
	controller.mutex.Unlock()

	setupPath := filepath.Join(controller.repository, "setup.ps1")
	commandArguments := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", setupPath, "-Distro", controller.distribution}
	commandArguments = append(commandArguments, arguments...)
	done, err := controller.runner.Start("powershell.exe", commandArguments)
	if err != nil {
		controller.finish(fmt.Sprintf("Could not start the host action: %v", err))
		return err
	}
	go func() {
		if err := <-done; err != nil {
			controller.finish(fmt.Sprintf("The PowerShell host action failed: %v", err))
			return
		}
		controller.finish("PowerShell completed. Refreshing host state.")
	}()
	return nil
}

func (controller *WindowsHostController) finish(message string) {
	controller.mutex.Lock()
	controller.busy = false
	controller.message = message
	controller.mutex.Unlock()
}

func (controller *WindowsHostController) Configure() error {
	return controller.start("-Yes")
}

func (controller *WindowsHostController) EnablePairing() error {
	return controller.start("-Pair")
}

func (controller *WindowsHostController) Revoke(ctx context.Context, fingerprint string) error {
	if runtime.GOOS != "windows" {
		return errors.New("client revocation is available only on Windows")
	}
	if !strings.HasPrefix(fingerprint, "SHA256:") || len(fingerprint) > 96 || strings.ContainsAny(fingerprint, "\r\n") {
		return errors.New("invalid client fingerprint")
	}
	encodedFingerprint := base64.StdEncoding.EncodeToString([]byte(fingerprint))
	script := strings.ReplaceAll(revokeClientScript, "__FINGERPRINT__", encodedFingerprint)
	output, err := controller.runner.Run(ctx, "wsl.exe", []string{"-d", controller.distribution, "--", "bash", "-s"}, script)
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = "Could not revoke the selected client."
		}
		return errors.New(message)
	}
	return nil
}

const revokeClientScript = `set -eu
fingerprint=$(printf '%s' '__FINGERPRINT__' | base64 --decode)
authorized_keys="$HOME/.ssh/authorized_keys"
[ -f "$authorized_keys" ] || { printf '%s\n' 'authorized_keys does not exist' >&2; exit 1; }
temporary=$(mktemp "$HOME/.ssh/.authorized_keys.XXXXXX")
trap 'rm -f "$temporary"' EXIT HUP INT TERM
removed=0
while IFS= read -r key || [ -n "$key" ]; do
  case "$key" in ''|'#'*) printf '%s\n' "$key" >> "$temporary"; continue ;; esac
  current=$(printf '%s\n' "$key" | ssh-keygen -lf - -E sha256 2>/dev/null | awk '{ print $2 }' || true)
  if [ "$current" = "$fingerprint" ]; then removed=1; else printf '%s\n' "$key" >> "$temporary"; fi
done < "$authorized_keys"
[ "$removed" -eq 1 ] || { printf '%s\n' 'selected client is no longer authorized' >&2; exit 1; }
chmod 600 "$temporary"
mv "$temporary" "$authorized_keys"
trap - EXIT HUP INT TERM
`
