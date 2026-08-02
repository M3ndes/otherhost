package dashboard

import (
	"errors"
	"os/exec"
	"runtime"
)

func OpenBrowser(address string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", address)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	case "linux":
		command = exec.Command("xdg-open", address)
	default:
		return errors.New("automatic browser opening is not supported on this platform")
	}
	return command.Start()
}
