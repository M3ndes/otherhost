package pairing

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func normalizeEd25519PublicKey(publicKey string) (string, error) {
	publicKey = strings.TrimSuffix(publicKey, "\n")
	publicKey = strings.TrimSuffix(publicKey, "\r")
	if strings.ContainsAny(publicKey, "\r\n") {
		return "", errors.New("SSH public key must contain exactly one line")
	}
	fields := strings.Fields(publicKey)
	if len(fields) < 2 || fields[0] != "ssh-ed25519" {
		return "", errors.New("pairing requires an Ed25519 SSH public key")
	}
	blob, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		return "", errors.New("SSH public key payload is not valid base64")
	}
	algorithm, remainder, ok := readSSHField(blob)
	if !ok || string(algorithm) != "ssh-ed25519" {
		return "", errors.New("SSH public key algorithm does not match its payload")
	}
	key, remainder, ok := readSSHField(remainder)
	if !ok || len(key) != 32 || len(remainder) != 0 {
		return "", errors.New("SSH public key payload is malformed")
	}
	return "ssh-ed25519 " + fields[1], nil
}

func readSSHField(value []byte) (field, remainder []byte, ok bool) {
	if len(value) < 4 {
		return nil, nil, false
	}
	size := int(binary.BigEndian.Uint32(value[:4]))
	if size < 0 || size > len(value)-4 {
		return nil, nil, false
	}
	return value[4 : 4+size], value[4+size:], true
}

func installKeyInFile(path, publicKey string) error {
	normalized, err := normalizeEd25519PublicKey(publicKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(existing), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == normalized {
			return os.Chmod(path, 0600)
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		if _, err := file.WriteString("\n"); err != nil {
			return err
		}
	}
	if _, err := file.WriteString(normalized + "\n"); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

func installKeyInWSL(distro, publicKey string) error {
	if runtime.GOOS != "windows" {
		return errors.New("WSL key installation is available only on Windows")
	}
	if !validDistroName(distro) {
		return errors.New("invalid WSL distribution name")
	}
	normalized, err := normalizeEd25519PublicKey(publicKey)
	if err != nil {
		return err
	}
	script := `set -eu
IFS= read -r public_key
umask 077
mkdir -p "$HOME/.ssh"
touch "$HOME/.ssh/authorized_keys"
chmod 700 "$HOME/.ssh"
chmod 600 "$HOME/.ssh/authorized_keys"
grep -Fqx -- "$public_key" "$HOME/.ssh/authorized_keys" || printf '%s\n' "$public_key" >> "$HOME/.ssh/authorized_keys"
`
	command := exec.Command("wsl.exe", "-d", distro, "--", "bash", "-c", script)
	command.Stdin = strings.NewReader(normalized + "\n")
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("could not install the SSH public key in WSL: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func readWSLHostKey(distro string) (string, error) {
	return readWSLHostKeyCommand(distro, "cat /etc/ssh/ssh_host_ed25519_key.pub")
}

func readWSLUserHostKey(distro string) (string, error) {
	return readWSLHostKeyCommand(distro, `cat "$HOME/.local/lib/devbox-bridge/ssh_host_ed25519_key.pub"`)
}

func readWSLHostKeyCommand(distro, script string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", errors.New("WSL host-key discovery is available only on Windows")
	}
	if !validDistroName(distro) {
		return "", errors.New("invalid WSL distribution name")
	}
	command := exec.Command("wsl.exe", "-d", distro, "--", "sh", "-c", script)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("could not read the WSL SSH host key: %s", strings.TrimSpace(string(output)))
	}
	return normalizeEd25519PublicKey(strings.TrimSpace(string(output)))
}

func validDistroName(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') &&
			!(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') &&
			!strings.ContainsRune("._ -", character) {
			return false
		}
	}
	return true
}

func knownHostsLine(host string, port int, publicKey string) (string, error) {
	normalized, err := normalizeEd25519PublicKey(publicKey)
	if err != nil {
		return "", err
	}
	if !validPort(port) {
		return "", errors.New("invalid SSH port")
	}
	return "[" + host + "]:" + strconv.Itoa(port) + " " + normalized, nil
}
