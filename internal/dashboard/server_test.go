package dashboard

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fixedCollector struct {
	snapshot Snapshot
}

func (collector fixedCollector) Collect() Snapshot { return collector.snapshot }

type recordingLauncher struct {
	alias string
	path  string
}

func (launcher *recordingLauncher) OpenProject(alias, projectPath string) error {
	launcher.alias = alias
	launcher.path = projectPath
	return nil
}

func TestSSHAliasMatchesExpectedOtherhostConfig(t *testing.T) {
	config := Config{
		Host:           "192.0.2.10",
		SSHUser:        "developer",
		SSHPort:        2222,
		IdentityFile:   "/Users/example/.ssh/otherhost key",
		KnownHostsFile: "/Users/example/.ssh/otherhost known hosts",
	}
	resolved := `host test-box
user developer
hostname 192.0.2.10
port 2222
identityfile /Users/example/.ssh/otherhost key
userknownhostsfile /Users/example/.ssh/otherhost known hosts
`
	if !sshAliasMatchesConfig(resolved, config) {
		t.Fatal("expected the managed SSH alias to match")
	}
}

func TestSSHAliasRejectsUnconfiguredHostname(t *testing.T) {
	config := Config{
		Host:           "192.0.2.10",
		SSHUser:        "developer",
		SSHPort:        2222,
		IdentityFile:   "/Users/example/.ssh/otherhost_ed25519",
		KnownHostsFile: "/Users/example/.ssh/otherhost_known_hosts",
	}
	resolved := `host test-box
user local-user
hostname test-box
port 22
identityfile ~/.ssh/id_ed25519
userknownhostsfile /Users/example/.ssh/known_hosts
`
	if sshAliasMatchesConfig(resolved, config) {
		t.Fatal("an unresolved hostname must not be accepted as the managed SSH alias")
	}
}

func TestDashboardServesEnglishEmbeddedUI(t *testing.T) {
	handler, err := NewHandler(fixedCollector{}, &recordingLauncher{}, "test-box")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Host = "127.0.0.1:7842"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	result := response.Result()
	body, _ := io.ReadAll(result.Body)
	if result.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", result.StatusCode)
	}
	page := string(body)
	for _, expected := range []string{`<html lang="en">`, "Build remotely.", "Projects", "Terminal", "Machine", `class="brand sidebar-brand"`, `<img src="/favicon.png" alt="">`, `/vendor/xterm/xterm.js`, `/vendor/addon-fit/addon-fit.js`} {
		if !strings.Contains(page, expected) {
			t.Fatalf("UI is missing %q", expected)
		}
	}
	if strings.Contains(page, "https://") || strings.Contains(page, "http://") {
		t.Fatal("UI contains a remote asset reference")
	}
	if !strings.Contains(result.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatal("content security policy is missing")
	}
}

func TestDashboardServesOtterFavicon(t *testing.T) {
	handler, err := NewHandler(fixedCollector{}, &recordingLauncher{}, "test-box")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/favicon.png", nil)
	request.Host = "127.0.0.1:7842"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "image/png") {
		t.Fatalf("unexpected favicon content type: %q", contentType)
	}
	if !bytes.HasPrefix(response.Body.Bytes(), []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		t.Fatal("favicon is not a PNG image")
	}
}

func TestDashboardBuffersTerminalInitializationOutput(t *testing.T) {
	handler, err := NewHandler(fixedCollector{}, &recordingLauncher{}, "test-box")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	request.Host = "127.0.0.1:7842"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	page := response.Body.String()
	for _, expected := range []string{
		"terminalStartupMarker",
		"findByteSequence",
		"Preparing terminal",
		"terminalStartupLimit",
		"terminalStartupTimeout",
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("terminal bootstrap buffering is missing %q", expected)
		}
	}
}

func TestDashboardTerminalUsesReadableLightPalette(t *testing.T) {
	handler, err := NewHandler(fixedCollector{}, &recordingLauncher{}, "test-box")
	if err != nil {
		t.Fatal(err)
	}
	for _, asset := range []struct {
		path     string
		expected []string
	}{
		{"/app.js", []string{
			"background: '#f7f7fa', foreground: '#24242b'",
			"black: '#24242b'",
			"white: '#5f5f69'",
		}},
		{"/app.css", []string{
			"--terminal-bg: #f7f7fa",
			"--terminal-text: #24242b",
			"--terminal-success: #18845c",
			"background: var(--terminal-bg)",
		}},
	} {
		request := httptest.NewRequest(http.MethodGet, asset.path, nil)
		request.Host = "127.0.0.1:7842"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d", asset.path, response.Code)
		}
		for _, expected := range asset.expected {
			if !strings.Contains(response.Body.String(), expected) {
				t.Fatalf("%s is missing terminal color %q", asset.path, expected)
			}
		}
	}
}

func TestDashboardRefreshesInventoryWhenProjectsOpen(t *testing.T) {
	handler, err := NewHandler(fixedCollector{}, &recordingLauncher{}, "test-box")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	request.Host = "127.0.0.1:7842"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	page := response.Body.String()
	for _, expected := range []string{
		"link.dataset.sectionLink === 'projects') refresh()",
		"document.addEventListener('visibilitychange'",
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("automatic project inventory refresh is missing %q", expected)
		}
	}
}

func TestDashboardProvidesBrandedEditorActions(t *testing.T) {
	handler, err := NewHandler(fixedCollector{}, &recordingLauncher{}, "test-box")
	if err != nil {
		t.Fatal(err)
	}
	for _, asset := range []struct {
		path     string
		expected []string
	}{
		{"/app.js", []string{
			"icons.codex",
			"icons.vscode",
			"<span>Codex</span>",
			"<span>VS Code</span>",
			"`Set up ${project.name} in Codex`",
			"`Open ${project.name} in VS Code`",
			"codex://settings/connections/ssh/add?name=",
			"Opening ${project.name} in VS Code.",
		}},
		{"/app.css", []string{".project-editor-actions", ".codex-project", ".vscode-project", ".codex-icon", ".vscode-icon"}},
	} {
		request := httptest.NewRequest(http.MethodGet, asset.path, nil)
		request.Host = "127.0.0.1:7842"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d", asset.path, response.Code)
		}
		for _, expected := range asset.expected {
			if !strings.Contains(response.Body.String(), expected) {
				t.Fatalf("%s is missing branded editor action %q", asset.path, expected)
			}
		}
	}
}

func TestSnapshotIncludesSSHAliasForCodexSetup(t *testing.T) {
	handler, err := NewHandler(fixedCollector{}, &recordingLauncher{}, "test-box")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/snapshot", nil)
	request.Host = "127.0.0.1:7842"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	var payload struct {
		SSHAlias string `json:"sshAlias"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.SSHAlias != "test-box" {
		t.Fatalf("unexpected SSH alias: %q", payload.SSHAlias)
	}
}

func TestDashboardServesProjectDeletionConfirmation(t *testing.T) {
	handler, err := NewHandler(fixedCollector{}, &recordingLauncher{}, "test-box")
	if err != nil {
		t.Fatal(err)
	}
	for _, asset := range []struct {
		path     string
		expected []string
	}{
		{"/", []string{"Permanent deletion", "Delete permanently", "data-delete-confirmation"}},
		{"/app.js", []string{"/api/projects/delete", "projectDeletionEnabled", "openDeleteProject"}},
		{"/app.css", []string{".delete-dialog", ".danger-button", ".delete-project"}},
	} {
		request := httptest.NewRequest(http.MethodGet, asset.path, nil)
		request.Host = "127.0.0.1:7842"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d", asset.path, response.Code)
		}
		for _, expected := range asset.expected {
			if !strings.Contains(response.Body.String(), expected) {
				t.Fatalf("%s is missing %q", asset.path, expected)
			}
		}
	}
}

func TestProjectActionsRequireInventoryTokenAndSameOrigin(t *testing.T) {
	projectPath := "/home/developer/src/app"
	collector := fixedCollector{snapshot: Snapshot{
		Status: "connected", Host: Host{Name: "test-box"}, UpdatedAt: time.Now(),
		Projects: []Project{{Name: "app", Path: projectPath}},
	}}
	launcher := &recordingLauncher{}
	handler, err := NewHandler(collector, launcher, "test-box")
	if err != nil {
		t.Fatal(err)
	}

	snapshotRequest := httptest.NewRequest(http.MethodGet, "/api/snapshot", nil)
	snapshotRequest.Host = "127.0.0.1:7842"
	snapshotResponse := httptest.NewRecorder()
	handler.ServeHTTP(snapshotResponse, snapshotRequest)
	var payload struct {
		ActionToken string `json:"actionToken"`
	}
	if err := json.NewDecoder(snapshotResponse.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.ActionToken == "" {
		t.Fatal("action token is missing")
	}

	requestBody := []byte(`{"path":"` + projectPath + `"}`)
	unauthorized := httptest.NewRequest(http.MethodPost, "/api/projects/open", bytes.NewReader(requestBody))
	unauthorized.Host = "127.0.0.1:7842"
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusForbidden {
		t.Fatalf("missing token returned %d", unauthorizedResponse.Code)
	}

	crossOrigin := httptest.NewRequest(http.MethodPost, "/api/projects/open", bytes.NewReader(requestBody))
	crossOrigin.Host = "127.0.0.1:7842"
	crossOrigin.Header.Set("Origin", "https://example.com")
	crossOrigin.Header.Set("X-Otherhost-Token", payload.ActionToken)
	crossOriginResponse := httptest.NewRecorder()
	handler.ServeHTTP(crossOriginResponse, crossOrigin)
	if crossOriginResponse.Code != http.StatusForbidden {
		t.Fatalf("cross-origin action returned %d", crossOriginResponse.Code)
	}

	authorized := httptest.NewRequest(http.MethodPost, "/api/projects/open", bytes.NewReader(requestBody))
	authorized.Host = "127.0.0.1:7842"
	authorized.Header.Set("Origin", "http://127.0.0.1:7842")
	authorized.Header.Set("X-Otherhost-Token", payload.ActionToken)
	authorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(authorizedResponse, authorized)
	if authorizedResponse.Code != http.StatusAccepted {
		t.Fatalf("authorized action returned %d: %s", authorizedResponse.Code, authorizedResponse.Body.String())
	}
	if launcher.alias != "test-box" || launcher.path != projectPath {
		t.Fatalf("unexpected launch: alias=%q path=%q", launcher.alias, launcher.path)
	}
}

func TestProjectActionRejectsPathOutsideLatestInventory(t *testing.T) {
	handler, err := NewHandler(fixedCollector{snapshot: Snapshot{Projects: []Project{}}}, &recordingLauncher{}, "test-box")
	if err != nil {
		t.Fatal(err)
	}
	snapshotResponse := httptest.NewRecorder()
	snapshotRequest := httptest.NewRequest(http.MethodGet, "/api/snapshot", nil)
	snapshotRequest.Host = "127.0.0.1:7842"
	handler.ServeHTTP(snapshotResponse, snapshotRequest)
	var payload struct {
		ActionToken string `json:"actionToken"`
	}
	_ = json.NewDecoder(snapshotResponse.Body).Decode(&payload)

	request := httptest.NewRequest(http.MethodPost, "/api/projects/open", strings.NewReader(`{"path":"/etc"}`))
	request.Host = "127.0.0.1:7842"
	request.Header.Set("Origin", "http://127.0.0.1:7842")
	request.Header.Set("X-Otherhost-Token", payload.ActionToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unlisted path returned %d", response.Code)
	}
}

func TestDashboardRejectsDNSRebindingHost(t *testing.T) {
	handler, err := NewHandler(fixedCollector{}, &recordingLauncher{}, "test-box")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/snapshot", nil)
	request.Host = "attacker.example"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMisdirectedRequest {
		t.Fatalf("untrusted host returned %d", response.Code)
	}
}
