package pairing

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDSZaD3EhKIadfnHAoP5FI2lDwzjk6xZ4H8vS2gFVrKe test-key"

func TestNumericComparisonMatchesAndBindsTranscript(t *testing.T) {
	clientPrivate, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hostPrivate, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	request := HelloRequest{
		Version: ProtocolVersion, Instance: "instance", ClientName: "MacBook",
		ClientPublicKey: encode(clientPrivate.PublicKey().Bytes()), ClientNonce: encode(make([]byte, 32)),
	}
	response := HelloResponse{
		SessionID: "session", HostName: "WINDOWS-PC",
		HostPublicKey: encode(hostPrivate.PublicKey().Bytes()), HostNonce: encode(make([]byte, 32)), ExpiresIn: 120,
	}
	transcript := makeTranscript("instance", request, response)
	clientKeys, err := deriveSessionKeys(clientPrivate, hostPrivate.PublicKey().Bytes(), transcript)
	if err != nil {
		t.Fatal(err)
	}
	hostKeys, err := deriveSessionKeys(hostPrivate, clientPrivate.PublicKey().Bytes(), transcript)
	if err != nil {
		t.Fatal(err)
	}
	if comparisonCode(clientKeys) != comparisonCode(hostKeys) {
		t.Fatal("both devices did not derive the same comparison code")
	}
	response.HostName = "ATTACKER-PC"
	tampered := makeTranscript("instance", request, response)
	tamperedKeys, err := deriveSessionKeys(clientPrivate, hostPrivate.PublicKey().Bytes(), tampered)
	if err != nil {
		t.Fatal(err)
	}
	if string(clientKeys.transcript) == string(tamperedKeys.transcript) {
		t.Fatal("device-name tampering did not change the authenticated transcript")
	}
}

func TestEncryptedMessagesRejectWrongSessionKey(t *testing.T) {
	key := make([]byte, 32)
	key[0] = 1
	aad := []byte("pairing transcript")
	nonce, ciphertext, err := seal(key, aad, PairPayload{SSHPublicKey: testPublicKey})
	if err != nil {
		t.Fatal(err)
	}
	var payload PairPayload
	if err := openSealed(key, aad, nonce, ciphertext, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SSHPublicKey != testPublicKey {
		t.Fatal("encrypted payload changed after decryption")
	}
	wrongKey := make([]byte, 32)
	wrongKey[0] = 2
	if err := openSealed(wrongKey, aad, nonce, ciphertext, &payload); err == nil {
		t.Fatal("encrypted payload was accepted with the wrong session key")
	}
}

func TestWSLKeyInstallArgumentsPreserveNormalizedPublicKey(t *testing.T) {
	arguments, err := wslKeyInstallArguments("Ubuntu", testPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(arguments) != 8 || arguments[0] != "-d" || arguments[1] != "Ubuntu" ||
		arguments[2] != "--" || arguments[3] != "bash" || arguments[4] != "-c" ||
		arguments[6] != "devbox-bridge" {
		t.Fatalf("unexpected WSL command arguments: %#v", arguments)
	}
	if !strings.Contains(arguments[5], `"$1"`) || strings.Contains(arguments[5], "IFS= read") {
		t.Fatalf("WSL install script does not read the encoded argument safely: %s", arguments[5])
	}
	decoded, err := base64.StdEncoding.DecodeString(arguments[7])
	if err != nil {
		t.Fatal(err)
	}
	expected := strings.Join(strings.Fields(testPublicKey)[:2], " ")
	if string(decoded) != expected {
		t.Fatalf("encoded WSL key changed after normalization: %q", decoded)
	}
	if strings.Contains(strings.Join(arguments, " "), testPublicKey) {
		t.Fatal("raw public key was embedded directly in the WSL command")
	}
}

func TestDefaultPairingPortsAvoidEphemeralRanges(t *testing.T) {
	address, err := net.ResolveUDPAddr("udp4", DefaultDiscoveryAddress)
	if err != nil {
		t.Fatal(err)
	}
	if address.Port != DefaultDiscoveryPort {
		t.Fatalf("discovery address port %d does not match default %d", address.Port, DefaultDiscoveryPort)
	}
	if DefaultDiscoveryPort >= 32768 || DefaultPairPort >= 32768 {
		t.Fatalf("default pairing ports must remain below common ephemeral ranges: UDP %d, TCP %d", DefaultDiscoveryPort, DefaultPairPort)
	}
	if DefaultDiscoveryPort == DefaultPairPort {
		t.Fatal("discovery and pairing must not share a port")
	}
}

func TestPairEndToEnd(t *testing.T) {
	port := reserveTCPPort(t)
	installed := make(chan string, 1)
	hostResult := make(chan error, 1)
	hostContext, cancelHost := context.WithCancel(context.Background())
	defer cancelHost()
	go func() {
		hostResult <- RunHost(hostContext, HostOptions{
			Instance: "test-instance", Name: "WINDOWS-PC", SSHUser: "developer",
			SSHPort: 2222, PairPort: port, Duration: 20 * time.Second,
			DisableDiscovery: true,
			Confirm:          func(string, string) bool { return true },
			InstallPublicKey: func(publicKey string) error { installed <- publicKey; return nil },
			ReadSSHHostPublicKey: func() (string, error) {
				return testPublicKey, nil
			},
		})
	}()
	waitForTCP(t, port)
	result, err := Pair(context.Background(), ClientOptions{
		Device:     Device{Instance: "test-instance", Name: "WINDOWS-PC", Address: "127.0.0.1", Port: port},
		ClientName: "MacBook", SSHPublicKey: testPublicKey,
		Confirm: func(string, string) bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Host != "127.0.0.1" || result.DevboxName != "WINDOWS-PC" || result.SSHUser != "developer" {
		t.Fatalf("unexpected pairing result: %+v", result)
	}
	select {
	case key := <-installed:
		if key != strings.Join(strings.Fields(testPublicKey)[:2], " ") {
			t.Fatalf("unexpected installed key: %s", key)
		}
	case <-time.After(time.Second):
		t.Fatal("host did not install the paired SSH key")
	}
	select {
	case err := <-hostResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("host did not close after successful pairing")
	}
}

func TestDirectDiscoveryFindsHostWithoutMulticast(t *testing.T) {
	port := reserveTCPPort(t)
	hostResult := make(chan error, 1)
	hostDiagnostics := make(chan string, 8)
	hostContext, cancelHost := context.WithCancel(context.Background())
	go func() {
		hostResult <- RunHost(hostContext, HostOptions{
			Instance: "direct-discovery", Name: "WINDOWS-PC", SSHUser: "developer",
			SSHPort: 2222, PairPort: port, Duration: 20 * time.Second,
			DisableDiscovery: true,
			Confirm:          func(string, string) bool { return true },
			InstallPublicKey: func(string) error { return nil },
			ReadSSHHostPublicKey: func() (string, error) {
				return testPublicKey, nil
			},
			Log: func(format string, arguments ...any) {
				hostDiagnostics <- fmt.Sprintf(format, arguments...)
			},
		})
	}()
	waitForTCP(t, port)
	waitForDiagnostic(t, hostDiagnostics, "TCP pairing listener active")
	discoveryContext, cancelDiscovery := context.WithTimeout(context.Background(), 2*time.Second)
	diagnostics := make([]string, 0, 1)
	devices := discoverAtAddresses(discoveryContext, []string{"127.0.0.1"}, port, func(format string, arguments ...any) {
		diagnostics = append(diagnostics, fmt.Sprintf(format, arguments...))
	})
	cancelDiscovery()
	if len(devices) != 1 {
		t.Fatalf("expected one directly discovered host, got %+v", devices)
	}
	if devices[0].Instance != "direct-discovery" || devices[0].Name != "WINDOWS-PC" ||
		devices[0].Address != "127.0.0.1" || devices[0].Port != port {
		t.Fatalf("unexpected directly discovered host: %+v", devices[0])
	}
	if output := strings.Join(diagnostics, "\n"); !strings.Contains(output, "1 endpoint(s) responded, 1 compatible devbox(es)") {
		t.Fatalf("direct discovery diagnostics were not actionable: %s", output)
	}
	waitForDiagnostic(t, hostDiagnostics, "direct discovery request accepted from 127.0.0.1")
	cancelHost()
	select {
	case err := <-hostResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected host shutdown result: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("host did not stop after direct discovery test")
	}
}

func TestDirectDiscoveryReportsUnreachableEndpoint(t *testing.T) {
	port := reserveTCPPort(t)
	diagnostics := make([]string, 0, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	devices := discoverAtAddresses(ctx, []string{"127.0.0.1"}, port, func(format string, arguments ...any) {
		diagnostics = append(diagnostics, fmt.Sprintf(format, arguments...))
	})
	if len(devices) != 0 {
		t.Fatalf("unexpected device on a closed port: %+v", devices)
	}
	output := strings.Join(diagnostics, "\n")
	if !strings.Contains(output, "1/1 probe(s)") ||
		!strings.Contains(output, "0 endpoint(s) responded, 0 compatible devbox(es)") {
		t.Fatalf("unreachable endpoint diagnostics were not actionable: %s", output)
	}
}

func TestUserScopedWSLRequiresDistribution(t *testing.T) {
	err := RunHost(context.Background(), HostOptions{
		Name: "WINDOWS-PC", SSHUser: "developer", SSHPort: 2222,
		PairPort: DefaultPairPort, Duration: 20 * time.Second, UserScopedWSL: true,
		Confirm: func(string, string) bool { return true },
	})
	if err == nil || !strings.Contains(err.Error(), "requires a WSL distribution") {
		t.Fatalf("unexpected validation result: %v", err)
	}
}

func TestRejectedComparisonDoesNotInstallKey(t *testing.T) {
	port := reserveTCPPort(t)
	installed := make(chan string, 1)
	hostResult := make(chan error, 1)
	hostContext, cancelHost := context.WithCancel(context.Background())
	defer cancelHost()
	go func() {
		hostResult <- RunHost(hostContext, HostOptions{
			Instance: "rejected-session", Name: "WINDOWS-PC", SSHUser: "developer",
			SSHPort: 2222, PairPort: port, Duration: 20 * time.Second,
			DisableDiscovery: true,
			Confirm:          func(string, string) bool { return true },
			InstallPublicKey: func(publicKey string) error { installed <- publicKey; return nil },
			ReadSSHHostPublicKey: func() (string, error) {
				return testPublicKey, nil
			},
		})
	}()
	waitForTCP(t, port)
	_, err := Pair(context.Background(), ClientOptions{
		Device:     Device{Instance: "rejected-session", Name: "WINDOWS-PC", Address: "127.0.0.1", Port: port},
		ClientName: "MacBook", SSHPublicKey: testPublicKey,
		Confirm: func(string, string) bool { return false },
	})
	if err == nil || !strings.Contains(err.Error(), "rejected on the Mac") {
		t.Fatalf("unexpected rejection result: %v", err)
	}
	select {
	case key := <-installed:
		t.Fatalf("rejected pairing installed a key: %s", key)
	default:
	}
	select {
	case err := <-hostResult:
		if err == nil || !strings.Contains(err.Error(), "rejected on the Mac") {
			t.Fatalf("unexpected host rejection result: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("host did not close after client rejection")
	}
}

func TestSaveClientConfigurationPinsHostIdentity(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "devbox.local.conf")
	knownHostsPath := filepath.Join(directory, "known_hosts")
	if err := os.WriteFile(configPath, []byte("# keep this comment\nhost=CHANGE_ME\nports=3000,8080\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result := ClientResult{
		PairResult: PairResult{
			DevboxName: "WINDOWS-PC", Host: "192.168.1.10", SSHUser: "developer", SSHPort: 2222, SSHHostKey: testPublicKey,
		},
	}
	if err := SaveClientConfiguration(configPath, knownHostsPath, result); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"# keep this comment", "host=192.168.1.10", "ports=3000,8080",
		"devbox_name=WINDOWS-PC", "ssh_user=developer", "ssh_port=2222",
		"known_hosts_file=" + knownHostsPath,
	} {
		if !strings.Contains(string(config), expected) {
			t.Fatalf("saved config is missing %q:\n%s", expected, config)
		}
	}
	knownHosts, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(knownHosts), "[192.168.1.10]:2222 ssh-ed25519 ") {
		t.Fatalf("unexpected known-hosts entry: %s", knownHosts)
	}
}

func reserveTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForTCP(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", stringPort(port)), 50*time.Millisecond)
		if err == nil {
			connection.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("pairing host did not start")
}

func waitForDiagnostic(t *testing.T, diagnostics <-chan string, expected string) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case message := <-diagnostics:
			if strings.Contains(message, expected) {
				return
			}
		case <-deadline:
			t.Fatalf("pairing host did not log %q", expected)
		}
	}
}

func stringPort(port int) string {
	return fmt.Sprintf("%d", port)
}
