package dashboard

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Name           string
	Host           string
	SSHUser        string
	SSHPort        int
	IdentityFile   string
	KnownHostsFile string
	ProjectsRoot   string
}

func LoadConfig(configPath string) (Config, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return Config{}, fmt.Errorf("could not open configuration: %w", err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimSuffix(scanner.Text(), "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		separator := strings.IndexByte(line, '=')
		if separator < 1 {
			continue
		}
		key := strings.TrimSpace(line[:separator])
		if _, exists := values[key]; !exists {
			values[key] = strings.TrimSpace(line[separator+1:])
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("could not read configuration: %w", err)
	}

	port, err := strconv.Atoi(values["ssh_port"])
	if err != nil || port < 1 || port > 65535 {
		return Config{}, errors.New("ssh_port must be between 1 and 65535")
	}
	root, err := normalizeProjectsRoot(values["projects_root"])
	if err != nil {
		return Config{}, err
	}
	config := Config{
		Name:           values["devbox_name"],
		Host:           values["host"],
		SSHUser:        values["ssh_user"],
		SSHPort:        port,
		IdentityFile:   resolveHomePath(values["identity_file"]),
		KnownHostsFile: resolveHomePath(values["known_hosts_file"]),
		ProjectsRoot:   root,
	}
	if !portableValue(config.Name, "._-") {
		return Config{}, errors.New("devbox_name contains unsupported characters")
	}
	if !portableValue(config.Host, ".:-") || config.Host == "CHANGE_ME" {
		return Config{}, errors.New("host contains unsupported characters")
	}
	if !portableValue(config.SSHUser, "._-") || config.SSHUser == "CHANGE_ME" {
		return Config{}, errors.New("ssh_user contains unsupported characters")
	}
	if config.IdentityFile == "" {
		return Config{}, errors.New("identity_file is required")
	}
	return config, nil
}

func normalizeProjectsRoot(value string) (string, error) {
	if value == "" {
		return "src", nil
	}
	if !strings.HasPrefix(value, "~/") {
		return "", errors.New("projects_root must be inside the remote WSL home directory")
	}
	relative := strings.TrimPrefix(value, "~/")
	if relative == "" || strings.ContainsAny(relative, "\r\n\x00") {
		return "", errors.New("projects_root is invalid")
	}
	for _, segment := range strings.Split(relative, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", errors.New("projects_root is invalid")
		}
	}
	return relative, nil
}

func resolveHomePath(value string) string {
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return value
	}
	return filepath.Join(home, value)
}

func portableValue(value, punctuation string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune(punctuation, character) {
			continue
		}
		return false
	}
	return true
}
