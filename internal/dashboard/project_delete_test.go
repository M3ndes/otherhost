package dashboard

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type recordingProjectDeleter struct {
	path  string
	calls int
	err   error
}

type blockingProjectDeleter struct {
	started chan struct{}
	release chan struct{}
}

func (deleter *blockingProjectDeleter) DeleteProject(string) error {
	close(deleter.started)
	<-deleter.release
	return nil
}

func (deleter *recordingProjectDeleter) DeleteProject(projectPath string) error {
	deleter.path = projectPath
	deleter.calls++
	return deleter.err
}

func TestRemoteProjectDeletionCommandTreatsPathAsData(t *testing.T) {
	projectPath := "/home/developer/src/$(touch /tmp/otherhost-delete-unsafe)"
	command := remoteProjectDeletionCommand(projectPath)
	if strings.Contains(command, projectPath) {
		t.Fatal("project path was interpolated into the remote shell command")
	}
	if !strings.Contains(command, base64.StdEncoding.EncodeToString([]byte(projectPath))) {
		t.Fatal("encoded project path is missing")
	}
	for _, expected := range []string{
		`case "$resolved" in`,
		`[ -d "$resolved/.git" ]`,
		`[ ! -L "$resolved/.git" ]`,
		`mountpoint -q -- "$resolved"`,
		`rm -rf -- "$resolved"`,
	} {
		if !strings.Contains(command, expected) {
			t.Fatalf("project deletion guard is missing %q", expected)
		}
	}
}

func TestRemoteProjectDeletionCommandDeletesEligibleCheckout(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	project := filepath.Join(home, "src", "project")
	unrelated := filepath.Join(home, "keep.txt")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("delete me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unrelated, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	runDeletionCommand(t, home, project, true)
	if _, err := os.Stat(project); !os.IsNotExist(err) {
		t.Fatalf("project still exists after deletion: %v", err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("unrelated file was changed: %v", err)
	}
}

func TestRemoteProjectDeletionCommandRejectsUnsafeTargets(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	outside := filepath.Join(root, "outside")
	worktree := filepath.Join(home, "linked-worktree")
	for _, directory := range []string{home, outside, worktree} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, directory := range []string{home, outside} {
		if err := os.MkdirAll(filepath.Join(directory, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{home, outside, worktree} {
		runDeletionCommand(t, home, target, false)
		if _, err := os.Stat(target); err != nil {
			t.Fatalf("rejected target was changed: %s: %v", target, err)
		}
	}
}

func TestProjectDeletionRequiresInventoryAuthorizationAndExactName(t *testing.T) {
	project := Project{Name: "app", Path: "/home/developer/src/app"}
	deleter := &recordingProjectDeleter{}
	handler, err := NewHandlerWithServices(
		fixedCollector{snapshot: Snapshot{Projects: []Project{project}}},
		&recordingLauncher{}, nil, deleter, "test-box",
	)
	if err != nil {
		t.Fatal(err)
	}
	token := loadActionToken(t, handler)

	unauthorized := projectDeletionRequest(project.Path, project.Name, "")
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusForbidden {
		t.Fatalf("unauthorized deletion returned %d", unauthorizedResponse.Code)
	}
	crossOrigin := projectDeletionRequest(project.Path, project.Name, token)
	crossOrigin.Header.Set("Origin", "https://attacker.example")
	crossOriginResponse := httptest.NewRecorder()
	handler.ServeHTTP(crossOriginResponse, crossOrigin)
	if crossOriginResponse.Code != http.StatusForbidden {
		t.Fatalf("cross-origin deletion returned %d", crossOriginResponse.Code)
	}

	unlisted := projectDeletionRequest("/home/developer/src/other", "other", token)
	unlistedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unlistedResponse, unlisted)
	if unlistedResponse.Code != http.StatusNotFound {
		t.Fatalf("unlisted deletion returned %d", unlistedResponse.Code)
	}

	wrongName := projectDeletionRequest(project.Path, "not-app", token)
	wrongNameResponse := httptest.NewRecorder()
	handler.ServeHTTP(wrongNameResponse, wrongName)
	if wrongNameResponse.Code != http.StatusBadRequest {
		t.Fatalf("incorrect confirmation returned %d", wrongNameResponse.Code)
	}

	authorized := projectDeletionRequest(project.Path, project.Name, token)
	authorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(authorizedResponse, authorized)
	if authorizedResponse.Code != http.StatusOK {
		t.Fatalf("authorized deletion returned %d: %s", authorizedResponse.Code, authorizedResponse.Body.String())
	}
	if deleter.calls != 1 || deleter.path != project.Path {
		t.Fatalf("unexpected deletion: calls=%d path=%q", deleter.calls, deleter.path)
	}

	repeated := projectDeletionRequest(project.Path, project.Name, token)
	repeatedResponse := httptest.NewRecorder()
	handler.ServeHTTP(repeatedResponse, repeated)
	if repeatedResponse.Code != http.StatusNotFound {
		t.Fatalf("repeated deletion returned %d", repeatedResponse.Code)
	}
}

func TestProjectDeletionFailureRestoresInventoryAuthorization(t *testing.T) {
	project := Project{Name: "app", Path: "/home/developer/src/app"}
	deleter := &recordingProjectDeleter{err: errors.New("remote deletion failed")}
	handler, err := NewHandlerWithServices(
		fixedCollector{snapshot: Snapshot{Projects: []Project{project}}},
		&recordingLauncher{}, nil, deleter, "test-box",
	)
	if err != nil {
		t.Fatal(err)
	}
	token := loadActionToken(t, handler)

	failedResponse := httptest.NewRecorder()
	handler.ServeHTTP(failedResponse, projectDeletionRequest(project.Path, project.Name, token))
	if failedResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("failed deletion returned %d", failedResponse.Code)
	}
	deleter.err = nil
	retryResponse := httptest.NewRecorder()
	handler.ServeHTTP(retryResponse, projectDeletionRequest(project.Path, project.Name, token))
	if retryResponse.Code != http.StatusOK {
		t.Fatalf("retry after failed deletion returned %d", retryResponse.Code)
	}
}

func TestProjectDeletionCannotRunTwiceConcurrently(t *testing.T) {
	project := Project{Name: "app", Path: "/home/developer/src/app"}
	deleter := &blockingProjectDeleter{started: make(chan struct{}), release: make(chan struct{})}
	handler, err := NewHandlerWithServices(
		fixedCollector{snapshot: Snapshot{Projects: []Project{project}}},
		&recordingLauncher{}, nil, deleter, "test-box",
	)
	if err != nil {
		t.Fatal(err)
	}
	token := loadActionToken(t, handler)
	firstResponse := httptest.NewRecorder()
	firstDone := make(chan struct{})
	go func() {
		handler.ServeHTTP(firstResponse, projectDeletionRequest(project.Path, project.Name, token))
		close(firstDone)
	}()
	<-deleter.started
	snapshotRequest := httptest.NewRequest(http.MethodGet, "/api/snapshot", nil)
	snapshotRequest.Host = "127.0.0.1:7842"
	snapshotResponse := httptest.NewRecorder()
	handler.ServeHTTP(snapshotResponse, snapshotRequest)
	var snapshot Snapshot
	if err := json.NewDecoder(snapshotResponse.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Projects) != 0 {
		t.Fatalf("project remained visible during deletion: %#v", snapshot.Projects)
	}

	secondResponse := httptest.NewRecorder()
	handler.ServeHTTP(secondResponse, projectDeletionRequest(project.Path, project.Name, token))
	if secondResponse.Code != http.StatusConflict {
		t.Fatalf("concurrent deletion returned %d", secondResponse.Code)
	}
	close(deleter.release)
	<-firstDone
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("first deletion returned %d", firstResponse.Code)
	}
}

func TestSnapshotReportsWhetherProjectDeletionIsEnabled(t *testing.T) {
	handler, err := NewHandlerWithServices(fixedCollector{}, &recordingLauncher{}, nil, &recordingProjectDeleter{}, "test-box")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/snapshot", nil)
	request.Host = "127.0.0.1:7842"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var payload struct {
		ProjectDeletionEnabled bool `json:"projectDeletionEnabled"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.ProjectDeletionEnabled {
		t.Fatal("snapshot did not advertise project deletion")
	}
}

func runDeletionCommand(t *testing.T, home, project string, wantSuccess bool) {
	t.Helper()
	command := exec.Command("bash", "-c", remoteProjectDeletionCommand(project))
	command.Env = append(os.Environ(), "HOME="+home)
	output, err := command.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("project deletion failed: %v\n%s", err, output)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("unsafe project deletion succeeded: %s", project)
	}
}

func projectDeletionRequest(projectPath, confirmation, token string) *http.Request {
	body, _ := json.Marshal(map[string]string{"path": projectPath, "confirmation": confirmation})
	request := httptest.NewRequest(http.MethodPost, "/api/projects/delete", bytes.NewReader(body))
	request.Host = "127.0.0.1:7842"
	request.Header.Set("Origin", "http://127.0.0.1:7842")
	request.Header.Set("X-Otherhost-Token", token)
	request.Header.Set("Content-Type", "application/json")
	return request
}
