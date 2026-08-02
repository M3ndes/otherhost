package dashboard

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const projectDeletionTimeout = 5 * time.Minute

type SSHProjectDeleter struct {
	config Config
}

func NewSSHProjectDeleter(config Config) *SSHProjectDeleter {
	return &SSHProjectDeleter{config: config}
}

func (deleter *SSHProjectDeleter) DeleteProject(projectPath string) error {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return errors.New("OpenSSH is not installed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), projectDeletionTimeout)
	defer cancel()
	arguments := append(sshConnectionArguments(deleter.config), sshTarget(deleter.config), remoteProjectDeletionCommand(projectPath))
	output, err := exec.CommandContext(ctx, sshPath, arguments...).CombinedOutput()
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return errors.New("project deletion timed out")
	}
	message := strings.TrimSpace(string(output))
	if message == "" {
		message = "the remote host could not delete the project"
	}
	return fmt.Errorf("could not delete project: %s", message)
}

func remoteProjectDeletionCommand(projectPath string) string {
	encodedPath := base64.StdEncoding.EncodeToString([]byte(projectPath))
	return `set -eu
fail() {
  printf '%s\n' "$1" >&2
  exit 1
}
project=$(printf '%s' '` + encodedPath + `' | base64 -d) || fail 'Could not decode the project path.'
[ -n "$project" ] || fail 'The project path is empty.'
[ -d "$project" ] || fail 'The project no longer exists.'
home=$(cd -P -- "$HOME" 2>/dev/null && pwd -P) || fail 'Could not resolve the WSL home directory.'
resolved=$(cd -P -- "$project" 2>/dev/null && pwd -P) || fail 'Could not resolve the project directory.'
case "$resolved" in
  "$home"/*) ;;
  *) fail 'The project is outside the WSL home directory.' ;;
esac
[ -d "$resolved/.git" ] && [ ! -L "$resolved/.git" ] || fail 'The path is not a primary Git checkout.'
if command -v mountpoint >/dev/null 2>&1 && mountpoint -q -- "$resolved"; then
  fail 'A mounted directory cannot be deleted as a project.'
fi
cd -- "$home"
rm -rf -- "$resolved"
if [ -e "$resolved" ] || [ -L "$resolved" ]; then
  fail 'The project directory could not be removed completely.'
fi`
}
