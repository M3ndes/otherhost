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
	for _, expected := range []string{`<html lang="en">`, "Build remotely.", "Projects", "Machine"} {
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
