package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/M3ndes/otherhost/internal/pairing"
)

var version = "development"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "[fail] %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		usage()
		return errors.New("a command is required")
	}
	contextWithSignals, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch os.Args[1] {
	case "host":
		return runHost(contextWithSignals, os.Args[2:])
	case "pair":
		return runPair(contextWithSignals, os.Args[2:])
	case "version", "--version", "-version":
		fmt.Printf("otherhost-pair %s\n", version)
		return nil
	case "help", "--help", "-h":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command: %s", os.Args[1])
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: otherhost-pair COMMAND [OPTIONS]

Commands:
  host     Make a Windows machine discoverable for two minutes.
  pair     Discover and pair with an Otherhost machine from macOS.
  version  Print the helper version.`)
}

func runHost(ctx context.Context, arguments []string) error {
	flags := flag.NewFlagSet("host", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	name := flags.String("name", "", "Windows device name")
	distro := flags.String("distro", "", "WSL distribution")
	sshUser := flags.String("ssh-user", "", "WSL SSH user")
	sshPort := flags.Int("ssh-port", 2222, "WSL SSH port")
	pairPort := flags.Int("pair-port", pairing.DefaultPairPort, "temporary pairing TCP port")
	discoveryAddress := flags.String("discovery-address", pairing.DefaultDiscoveryAddress, "local multicast discovery address")
	duration := flags.Duration("duration", 2*time.Minute, "discoverable duration")
	authorizedKeys := flags.String("authorized-keys", "", "authorized_keys path for development")
	hostKey := flags.String("ssh-host-key", "", "SSH host public key for development")
	userScopedWSL := flags.Bool("user-scoped-wsl", false, "use the user-scoped WSL SSH host")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("host does not accept positional arguments")
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Otherhost\n\nPairing is enabled for %s.\n", duration.Round(time.Second))
	fmt.Printf("[diag] Pairing helper version: %s\n", version)
	fmt.Println("Waiting for a Mac on this private network...")
	err := pairing.RunHost(ctx, pairing.HostOptions{
		Name: *name, Distro: *distro, SSHUser: *sshUser, SSHPort: *sshPort,
		PairPort: *pairPort, DiscoveryAddress: *discoveryAddress, Duration: *duration,
		AuthorizedKeysFile: *authorizedKeys, SSHHostPublicKey: *hostKey,
		UserScopedWSL: *userScopedWSL, Log: printDiagnostic,
		Confirm: func(clientName, code string) bool {
			fmt.Printf("\n%s wants to connect.\n\n", clientName)
			fmt.Printf("Pairing code: %s\n\n", formatCode(code))
			return promptYesNo(reader, "Does the Mac show the same code? [y/N] ")
		},
	})
	if err != nil {
		if errors.Is(err, pairing.ErrPairingExpired) {
			fmt.Println("\n[ok] Pairing mode expired")
			fmt.Println("[ok] Pairing listener stopped")
			return nil
		}
		if errors.Is(err, pairing.ErrRejectedOnWindows) || errors.Is(err, pairing.ErrRejectedOnMac) {
			fmt.Println("\n[ok] Pairing cancelled")
			fmt.Println("[ok] Pairing listener stopped")
			return nil
		}
		return err
	}
	fmt.Println("\n[ok] Devices verified")
	fmt.Println("[ok] Mac SSH public key installed")
	fmt.Println("[ok] Pairing mode disabled")
	return nil
}

func runPair(ctx context.Context, arguments []string) error {
	flags := flag.NewFlagSet("pair", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	configPath := flags.String("config", "", "otherhost configuration path")
	publicKeyPath := flags.String("public-key", "", "Mac SSH public-key path")
	knownHostsPath := flags.String("known-hosts", "", "dedicated known-hosts path")
	pairPort := flags.Int("pair-port", pairing.DefaultPairPort, "temporary pairing TCP port")
	discoveryAddress := flags.String("discovery-address", pairing.DefaultDiscoveryAddress, "local multicast discovery address")
	discoveryTimeout := flags.Duration("discovery-timeout", 5*time.Second, "device search duration")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("pair does not accept positional arguments")
	}
	if *configPath == "" || *publicKeyPath == "" || *knownHostsPath == "" {
		return errors.New("--config, --public-key, and --known-hosts are required")
	}
	publicKey, err := os.ReadFile(*publicKeyPath)
	if err != nil {
		return fmt.Errorf("could not read the Mac SSH public key: %w", err)
	}
	clientName, err := os.Hostname()
	if err != nil {
		return errors.New("could not determine the Mac device name")
	}
	clientName = portableDeviceName(clientName)

	fmt.Println("Searching for Otherhost machines...")
	fmt.Printf("[diag] Pairing helper version: %s\n", version)
	discoveryContext, cancel := context.WithTimeout(ctx, *discoveryTimeout)
	devices, err := pairing.DiscoverDevices(discoveryContext, *discoveryAddress, *pairPort, printDiagnostic)
	cancel()
	if err != nil {
		return fmt.Errorf("local discovery failed: %w", err)
	}
	if len(devices) == 0 {
		return errors.New("no Otherhost machine was found; enable pairing on Windows and keep both devices on the same private network")
	}
	reader := bufio.NewReader(os.Stdin)
	device, err := selectDevice(reader, devices)
	if err != nil {
		return err
	}
	fmt.Printf("[ok] Found %s at %s:%d\n", device.Name, device.Address, device.Port)
	result, err := pairing.Pair(ctx, pairing.ClientOptions{
		Device: device, ClientName: clientName, SSHPublicKey: string(publicKey),
		Confirm: func(hostName, code string) bool {
			fmt.Printf("\nPairing code: %s\n\n", formatCode(code))
			return promptYesNo(reader, "Does Windows show the same code? [y/N] ")
		},
	})
	if err != nil {
		return err
	}
	if err := pairing.SaveClientConfiguration(*configPath, *knownHostsPath, result); err != nil {
		return err
	}
	fmt.Println("[ok] Devices verified")
	fmt.Println("[ok] SSH public key installed")
	fmt.Println("[ok] SSH host identity pinned")
	fmt.Println("[ok] Connection saved")
	return nil
}

func printDiagnostic(format string, arguments ...any) {
	fmt.Printf("[diag] "+format+"\n", arguments...)
}

func selectDevice(reader *bufio.Reader, devices []pairing.Device) (pairing.Device, error) {
	if len(devices) == 1 {
		return devices[0], nil
	}
	fmt.Println("\nOtherhost machines found:")
	for index, device := range devices {
		fmt.Printf("  [%d] %s\n", index+1, device.Name)
	}
	fmt.Print("Select a host: ")
	answer, err := reader.ReadString('\n')
	if err != nil {
		return pairing.Device{}, errors.New("could not read the device selection")
	}
	selection, err := strconv.Atoi(strings.TrimSpace(answer))
	if err != nil || selection < 1 || selection > len(devices) {
		return pairing.Device{}, errors.New("invalid device selection")
	}
	return devices[selection-1], nil
}

func promptYesNo(reader *bufio.Reader, prompt string) bool {
	fmt.Print(prompt)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

func formatCode(code string) string {
	if len(code) == 6 {
		return code[:3] + " " + code[3:]
	}
	return code
}

func portableDeviceName(value string) string {
	var result strings.Builder
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("._ -", character) {
			result.WriteRune(character)
		} else {
			result.WriteByte('-')
		}
		if result.Len() == 64 {
			break
		}
	}
	cleaned := strings.TrimSpace(result.String())
	if cleaned == "" {
		return "Mac"
	}
	return cleaned
}
