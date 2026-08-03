package dashboard

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

const dashboardTestPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDSZaD3EhKIadfnHAoP5FI2lDwzjk6xZ4H8vS2gFVrKe test-mac"

func hostReportLine(key, value string) string {
	return fmt.Sprintf("%s %s", key, base64.StdEncoding.EncodeToString([]byte(value)))
}

func TestParseHostReportBuildsSetupClientsAndSessions(t *testing.T) {
	snapshot := Snapshot{Setup: defaultHostSetup(), Clients: []Client{}, Sessions: []Session{}}
	report := strings.Join([]string{
		hostReportLine("setup.otherhost", "ready"),
		hostReportLine("setup.ssh", "ready"),
		hostReportLine("setup.docker", "optional"),
		hostReportLine("environment.distribution", "Ubuntu 24.04 LTS"),
		hostReportLine("environment.processors", "24"),
		hostReportLine("client.key", dashboardTestPublicKey),
		hostReportLine("session.address", "192.0.2.12:53184"),
	}, "\n")
	parseHostReport(report, &snapshot)

	if snapshot.Setup.State != "ready" || snapshot.Environment.Processors != 24 {
		t.Fatalf("unexpected host report: %+v", snapshot)
	}
	if len(snapshot.Clients) != 1 || snapshot.Clients[0].Name != "test-mac" || snapshot.Clients[0].Fingerprint != "SHA256:Tv2nUYxqO8E39QmW267jemkqJq7sbpciWkRgxj9ttac" {
		t.Fatalf("unexpected authorized client: %+v", snapshot.Clients)
	}
	if len(snapshot.Sessions) != 1 || snapshot.Sessions[0].Address != "192.0.2.12:53184" {
		t.Fatalf("unexpected active sessions: %+v", snapshot.Sessions)
	}
}

func TestHostInventoryEncodesNamesAndValuesAsSeparateFields(t *testing.T) {
	script := hostInventoryScript()
	if !strings.Contains(script, `printf '%s ' "$1"`) || !strings.Contains(script, `printf '%s' "$2" | base64`) {
		t.Fatal("host report encoding does not preserve the field name and encoded value")
	}
}

func TestRevokeScriptUsesFingerprintAndAtomicAuthorizedKeysReplacement(t *testing.T) {
	for _, expected := range []string{"ssh-keygen -lf", `current=$(printf`, `mv "$temporary" "$authorized_keys"`, `chmod 600 "$temporary"`} {
		if !strings.Contains(revokeClientScript, expected) {
			t.Fatalf("revocation script is missing %q", expected)
		}
	}
}
