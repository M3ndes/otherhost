package dashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRemoteTerminalCommandQuotesInventoryPath(t *testing.T) {
	projectPath := "/home/developer/src/it's $(not executable)"
	command := remoteTerminalCommand(projectPath)
	expected := `cd -- '/home/developer/src/it'"'"'s $(not executable)' && unset SSH_CLIENT SSH_CONNECTION SSH_TTY; exec "${SHELL:-/bin/bash}" -l`
	if command != expected {
		t.Fatalf("unexpected remote command:\n%s", command)
	}
}

func TestNormalizeTerminalSizeAppliesDefaultsAndBounds(t *testing.T) {
	size, err := normalizeTerminalSize(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if size.Columns != defaultTerminalColumns || size.Rows != defaultTerminalRows {
		t.Fatalf("unexpected default size: %+v", size)
	}
	if _, err := normalizeTerminalSize(maxTerminalColumns+1, defaultTerminalRows); err == nil {
		t.Fatal("oversized terminal was accepted")
	}
}

func TestSSHConnectionAlwaysRequiresStrictHostKeyChecking(t *testing.T) {
	arguments := sshConnectionArguments(Config{
		Host: "192.0.2.10", SSHUser: "developer", SSHPort: 2222, IdentityFile: "/tmp/test-key",
	})
	joined := strings.Join(arguments, " ")
	if !strings.Contains(joined, "StrictHostKeyChecking=yes") {
		t.Fatalf("SSH arguments do not fail closed: %s", joined)
	}
}

func TestTerminalCreationRequiresAuthorizationAndInventory(t *testing.T) {
	projectPath := "/home/developer/src/app"
	launcher := newFakeTerminalLauncher()
	handler, err := NewHandlerWithTerminal(fixedCollector{snapshot: Snapshot{
		Projects: []Project{{Name: "app", Path: projectPath}},
	}}, &recordingLauncher{}, launcher, "test-box")
	if err != nil {
		t.Fatal(err)
	}
	token := loadActionToken(t, handler)

	unauthorized := httptest.NewRequest(http.MethodPost, "/api/terminals", strings.NewReader(`{"path":""}`))
	unauthorized.Host = "127.0.0.1:7842"
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusForbidden {
		t.Fatalf("unauthorized terminal returned %d", unauthorizedResponse.Code)
	}

	outside := terminalCreationRequest(`/etc`, token)
	outsideResponse := httptest.NewRecorder()
	handler.ServeHTTP(outsideResponse, outside)
	if outsideResponse.Code != http.StatusNotFound {
		t.Fatalf("unlisted terminal path returned %d", outsideResponse.Code)
	}

	home := terminalCreationRequest("", token)
	homeResponse := httptest.NewRecorder()
	handler.ServeHTTP(homeResponse, home)
	if homeResponse.Code != http.StatusCreated {
		t.Fatalf("home terminal returned %d: %s", homeResponse.Code, homeResponse.Body.String())
	}
	var session terminalSessionResponse
	if err := json.NewDecoder(homeResponse.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(session.Protocol, terminalProtocolPrefix) || !strings.HasSuffix(session.SocketPath, "/socket") {
		t.Fatalf("unexpected terminal session: %+v", session)
	}
	if strings.Contains(session.SocketPath, strings.TrimPrefix(session.Protocol, terminalProtocolPrefix)) {
		t.Fatal("terminal secret leaked into the WebSocket URL")
	}
}

func TestTerminalCreationLimitsPendingSessionsAndStopsDuringShutdown(t *testing.T) {
	handler, err := NewHandlerWithTerminal(fixedCollector{}, &recordingLauncher{}, newFakeTerminalLauncher(), "test-box")
	if err != nil {
		t.Fatal(err)
	}
	token := loadActionToken(t, handler)
	for index := 0; index < maxTerminalSessions; index++ {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, terminalCreationRequest("", token))
		if response.Code != http.StatusCreated {
			t.Fatalf("terminal %d returned %d", index+1, response.Code)
		}
	}

	limited := httptest.NewRecorder()
	handler.ServeHTTP(limited, terminalCreationRequest("", token))
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("terminal limit returned %d", limited.Code)
	}

	handler.CloseTerminals()
	shuttingDown := httptest.NewRecorder()
	handler.ServeHTTP(shuttingDown, terminalCreationRequest("", token))
	if shuttingDown.Code != http.StatusServiceUnavailable {
		t.Fatalf("shutdown terminal returned %d", shuttingDown.Code)
	}
}

func TestTerminalWebSocketIsOneTimeAndBridgesInputOutputAndResize(t *testing.T) {
	projectPath := "/home/developer/src/app"
	launcher := newFakeTerminalLauncher()
	handler, err := NewHandlerWithTerminal(fixedCollector{snapshot: Snapshot{
		Projects: []Project{{Name: "app", Path: projectPath}},
	}}, &recordingLauncher{}, launcher, "test-box")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	token := loadActionTokenFromServer(t, server.URL)
	session := createTerminalSessionFromServer(t, server.URL, projectPath, token)
	webSocketURL := "ws" + strings.TrimPrefix(server.URL, "http") + session.SocketPath
	header := http.Header{"Origin": []string{server.URL}}

	wrongConnection, wrongResponse, wrongError := websocket.DefaultDialer.Dial(webSocketURL, headerWithProtocol(header, terminalProtocolPrefix+"wrong"))
	if wrongConnection != nil {
		wrongConnection.Close()
	}
	if wrongError == nil || wrongResponse == nil || wrongResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong protocol was not rejected: response=%v error=%v", wrongResponse, wrongError)
	}

	dialer := websocket.Dialer{Subprotocols: []string{session.Protocol}}
	connection, _, err := dialer.Dial(webSocketURL, header)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if connection.Subprotocol() != session.Protocol {
		t.Fatalf("unexpected negotiated protocol: %q", connection.Subprotocol())
	}

	terminal := launcher.waitForTerminal(t)
	if launcher.path != projectPath {
		t.Fatalf("terminal started in %q", launcher.path)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, []byte("pwd\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case input := <-terminal.input:
		if string(input) != "pwd\n" {
			t.Fatalf("unexpected terminal input: %q", input)
		}
	case <-time.After(time.Second):
		t.Fatal("terminal input was not forwarded")
	}

	if err := connection.WriteJSON(map[string]any{"type": "resize", "columns": 132, "rows": 41}); err != nil {
		t.Fatal(err)
	}
	select {
	case size := <-terminal.resized:
		if size.Columns != 132 || size.Rows != 41 {
			t.Fatalf("unexpected resize: %+v", size)
		}
	case <-time.After(time.Second):
		t.Fatal("terminal resize was not forwarded")
	}

	if _, err := terminal.output.Write([]byte("hello from WSL")); err != nil {
		t.Fatal(err)
	}
	messageType, output, err := connection.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if messageType != websocket.BinaryMessage || string(output) != "hello from WSL" {
		t.Fatalf("unexpected terminal output: type=%d output=%q", messageType, output)
	}

	secondConnection, secondResponse, secondError := dialer.Dial(webSocketURL, header)
	if secondConnection != nil {
		secondConnection.Close()
	}
	if secondError == nil || secondResponse == nil || secondResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("terminal authorization was reusable: response=%v error=%v", secondResponse, secondError)
	}

	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-terminal.closed:
	case <-time.After(time.Second):
		t.Fatal("terminal process remained open after the WebSocket closed")
	}
}

func TestTerminalWebSocketRequiresSameOrigin(t *testing.T) {
	launcher := newFakeTerminalLauncher()
	handler, err := NewHandlerWithTerminal(fixedCollector{}, &recordingLauncher{}, launcher, "test-box")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	token := loadActionTokenFromServer(t, server.URL)
	session := createTerminalSessionFromServer(t, server.URL, "", token)

	dialer := websocket.Dialer{Subprotocols: []string{session.Protocol}}
	connection, response, dialError := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+session.SocketPath,
		http.Header{"Origin": []string{"https://attacker.example"}})
	if connection != nil {
		connection.Close()
	}
	if dialError == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin terminal was not rejected: response=%v error=%v", response, dialError)
	}
}

type terminalSessionResponse struct {
	SocketPath string `json:"socketPath"`
	Protocol   string `json:"protocol"`
}

func loadActionToken(t *testing.T, handler http.Handler) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/snapshot", nil)
	request.Host = "127.0.0.1:7842"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var payload struct {
		ActionToken string `json:"actionToken"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload.ActionToken
}

func loadActionTokenFromServer(t *testing.T, serverURL string) string {
	t.Helper()
	response, err := http.Get(serverURL + "/api/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var payload struct {
		ActionToken string `json:"actionToken"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload.ActionToken
}

func terminalCreationRequest(projectPath, token string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/terminals", strings.NewReader(`{"path":`+jsonString(projectPath)+`}`))
	request.Host = "127.0.0.1:7842"
	request.Header.Set("Origin", "http://127.0.0.1:7842")
	request.Header.Set("X-Otherhost-Token", token)
	request.Header.Set("Content-Type", "application/json")
	return request
}

func createTerminalSessionFromServer(t *testing.T, serverURL, projectPath, token string) terminalSessionResponse {
	t.Helper()
	body := bytes.NewBufferString(`{"path":` + jsonString(projectPath) + `,"columns":100,"rows":28}`)
	request, err := http.NewRequest(http.MethodPost, serverURL+"/api/terminals", body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", serverURL)
	request.Header.Set("X-Otherhost-Token", token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		contents, _ := io.ReadAll(response.Body)
		t.Fatalf("terminal creation returned %d: %s", response.StatusCode, contents)
	}
	var session terminalSessionResponse
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	return session
}

func jsonString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func headerWithProtocol(header http.Header, protocol string) http.Header {
	copy := header.Clone()
	copy.Set("Sec-WebSocket-Protocol", protocol)
	return copy
}

type fakeTerminalLauncher struct {
	started chan *fakeTerminal
	path    string
	size    TerminalSize
}

func newFakeTerminalLauncher() *fakeTerminalLauncher {
	return &fakeTerminalLauncher{started: make(chan *fakeTerminal, 1)}
}

func (launcher *fakeTerminalLauncher) StartTerminal(projectPath string, size TerminalSize) (Terminal, error) {
	terminal := newFakeTerminal()
	launcher.path = projectPath
	launcher.size = size
	launcher.started <- terminal
	return terminal, nil
}

func (launcher *fakeTerminalLauncher) waitForTerminal(t *testing.T) *fakeTerminal {
	t.Helper()
	select {
	case terminal := <-launcher.started:
		return terminal
	case <-time.After(time.Second):
		t.Fatal("terminal did not start")
		return nil
	}
}

type fakeTerminal struct {
	reader    *io.PipeReader
	output    *io.PipeWriter
	input     chan []byte
	resized   chan TerminalSize
	closed    chan struct{}
	closeOnce sync.Once
}

func newFakeTerminal() *fakeTerminal {
	reader, writer := io.Pipe()
	return &fakeTerminal{
		reader: reader, output: writer, input: make(chan []byte, 4), resized: make(chan TerminalSize, 4), closed: make(chan struct{}),
	}
}

func (terminal *fakeTerminal) Read(buffer []byte) (int, error) {
	return terminal.reader.Read(buffer)
}

func (terminal *fakeTerminal) Write(buffer []byte) (int, error) {
	copy := append([]byte(nil), buffer...)
	terminal.input <- copy
	return len(buffer), nil
}

func (terminal *fakeTerminal) Resize(size TerminalSize) error {
	terminal.resized <- size
	return nil
}

func (terminal *fakeTerminal) Close() error {
	var closeError error
	terminal.closeOnce.Do(func() {
		close(terminal.closed)
		closeError = errors.Join(terminal.output.Close(), terminal.reader.Close())
	})
	return closeError
}
