package pairing

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func SaveClientConfiguration(configPath, knownHostsPath string, result ClientResult) error {
	if strings.ContainsAny(result.Host, "\r\n=") || netHostInvalid(result.Host) {
		return errors.New("invalid paired host address")
	}
	if strings.ContainsAny(knownHostsPath, "\r\n\"") {
		return errors.New("invalid known-hosts path")
	}
	line, err := knownHostsLine(result.Host, result.SSHPort, result.SSHHostKey)
	if err != nil {
		return err
	}
	if err := writeAtomic(knownHostsPath, []byte(line+"\n"), 0600); err != nil {
		return fmt.Errorf("could not save the pinned SSH host key: %w", err)
	}
	values := map[string]string{
		"devbox_name":      result.DevboxName,
		"host":             result.Host,
		"ssh_user":         result.SSHUser,
		"ssh_port":         fmt.Sprintf("%d", result.SSHPort),
		"known_hosts_file": knownHostsPath,
	}
	if err := updateKeyValueFile(configPath, values); err != nil {
		return fmt.Errorf("could not save the devbox configuration: %w", err)
	}
	return nil
}

func updateKeyValueFile(path string, values map[string]string) error {
	for key, value := range values {
		if key == "" || strings.ContainsAny(key, "=\r\n") || strings.ContainsAny(value, "\r\n") {
			return errors.New("configuration contains an invalid key or value")
		}
	}
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	newline := "\n"
	if bytesContain(content, []byte("\r\n")) {
		newline = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	seen := map[string]bool{}
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		separator := strings.Index(trimmed, "=")
		if separator < 1 {
			continue
		}
		key := strings.TrimSpace(trimmed[:separator])
		if value, ok := values[key]; ok {
			lines[index] = key + "=" + value
			seen[key] = true
		}
	}
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	for _, key := range []string{"devbox_name", "host", "ssh_user", "ssh_port", "known_hosts_file"} {
		if !seen[key] {
			lines = append(lines, key+"="+values[key])
		}
	}
	output := strings.Join(lines, newline)
	if !strings.HasSuffix(output, newline) {
		output += newline
	}
	return writeAtomic(path, []byte(output), 0600)
}

func writeAtomic(path string, content []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".devbox-pair-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func bytesContain(value, needle []byte) bool {
	return strings.Contains(string(value), string(needle))
}

func netHostInvalid(value string) bool {
	if value == "" {
		return true
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') &&
			!(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') &&
			!strings.ContainsRune(".:-", character) {
			return true
		}
	}
	return false
}
