package pairing

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestWSLTemporaryScriptHandoff(t *testing.T) {
	if runtime.GOOS != "windows" || os.Getenv("DEVBOX_WSL_INTEGRATION_TEST") != "1" {
		t.Skip("set DEVBOX_WSL_INTEGRATION_TEST=1 on Windows to test the live WSL handoff")
	}
	distro := os.Getenv("DEVBOX_WSL_DISTRO")
	if distro == "" {
		distro = "Ubuntu"
	}
	const marker = "devbox-wsl-script-handoff-ok"
	output, err := runWSLScript(distro, "devbox-pair-integration", "printf '%s\\n' '"+marker+"'\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(output)) != marker {
		t.Fatalf("unexpected WSL script output: %q", output)
	}
}
