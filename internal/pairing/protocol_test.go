package pairing

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
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
		})
	}()
	waitForTCP(t, port)
	discoveryContext, cancelDiscovery := context.WithTimeout(context.Background(), 2*time.Second)
	devices := discoverAtAddresses(discoveryContext, []string{"127.0.0.1"}, port)
	cancelDiscovery()
	if len(devices) != 1 {
		t.Fatalf("expected one directly discovered host, got %+v", devices)
	}
	if devices[0].Instance != "direct-discovery" || devices[0].Name != "WINDOWS-PC" ||
		devices[0].Address != "127.0.0.1" || devices[0].Port != port {
		t.Fatalf("unexpected directly discovered host: %+v", devices[0])
	}
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

func TestUserScopedWSLRequiresDistribution(t *testing.T) {
	err := RunHost(context.Background(), HostOptions{
		Name: "WINDOWS-PC", SSHUser: "developer", SSHPort: 2222,
		PairPort: 45871, Duration: 20 * time.Second, UserScopedWSL: true,
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

func stringPort(port int) string {
	return fmt.Sprintf("%d", port)
}
