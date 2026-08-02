package dashboard

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/creack/pty"
)

const (
	defaultTerminalColumns = 120
	defaultTerminalRows    = 32
	minTerminalColumns     = 20
	maxTerminalColumns     = 400
	minTerminalRows        = 5
	maxTerminalRows        = 200
)

type TerminalSize struct {
	Columns uint16
	Rows    uint16
}

type Terminal interface {
	io.ReadWriteCloser
	Resize(TerminalSize) error
}

type TerminalLauncher interface {
	StartTerminal(projectPath string, size TerminalSize) (Terminal, error)
}

type SSHTerminalLauncher struct {
	config Config
}

func NewSSHTerminalLauncher(config Config) *SSHTerminalLauncher {
	return &SSHTerminalLauncher{config: config}
}

func (launcher *SSHTerminalLauncher) StartTerminal(projectPath string, size TerminalSize) (Terminal, error) {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return nil, errors.New("OpenSSH is not installed")
	}
	if !validTerminalSize(size) {
		return nil, errors.New("terminal size is outside the supported range")
	}

	arguments := append(sshConnectionArguments(launcher.config), "-tt", sshTarget(launcher.config), remoteTerminalCommand(projectPath))
	command := exec.Command(sshPath, arguments...)
	command.Env = append(os.Environ(), "TERM=xterm-256color")
	terminalFile, err := pty.StartWithSize(command, &pty.Winsize{Rows: size.Rows, Cols: size.Columns})
	if err != nil {
		return nil, fmt.Errorf("could not start the remote terminal: %w", err)
	}

	process := &sshTerminal{file: terminalFile, command: command, done: make(chan struct{})}
	go func() {
		_ = command.Wait()
		close(process.done)
	}()
	return process, nil
}

func validTerminalSize(size TerminalSize) bool {
	return size.Columns >= minTerminalColumns && size.Columns <= maxTerminalColumns &&
		size.Rows >= minTerminalRows && size.Rows <= maxTerminalRows
}

func normalizeTerminalSize(columns, rows int) (TerminalSize, error) {
	if columns == 0 {
		columns = defaultTerminalColumns
	}
	if rows == 0 {
		rows = defaultTerminalRows
	}
	if columns < minTerminalColumns || columns > maxTerminalColumns || rows < minTerminalRows || rows > maxTerminalRows {
		return TerminalSize{}, errors.New("terminal size is outside the supported range")
	}
	return TerminalSize{Columns: uint16(columns), Rows: uint16(rows)}, nil
}

func remoteTerminalCommand(projectPath string) string {
	loginShell := `exec "${SHELL:-/bin/bash}" -l`
	if projectPath == "" {
		return loginShell
	}
	return "cd -- " + quotePOSIXShell(projectPath) + " && " + loginShell
}

func quotePOSIXShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

type sshTerminal struct {
	file      *os.File
	command   *exec.Cmd
	done      chan struct{}
	closeOnce sync.Once
}

func (terminal *sshTerminal) Read(buffer []byte) (int, error) {
	return terminal.file.Read(buffer)
}

func (terminal *sshTerminal) Write(buffer []byte) (int, error) {
	return terminal.file.Write(buffer)
}

func (terminal *sshTerminal) Resize(size TerminalSize) error {
	if !validTerminalSize(size) {
		return errors.New("terminal size is outside the supported range")
	}
	return pty.Setsize(terminal.file, &pty.Winsize{Rows: size.Rows, Cols: size.Columns})
}

func (terminal *sshTerminal) Close() error {
	var closeError error
	terminal.closeOnce.Do(func() {
		closeError = terminal.file.Close()
		select {
		case <-terminal.done:
		default:
			if terminal.command.Process != nil {
				_ = terminal.command.Process.Kill()
			}
		}
	})
	return closeError
}
