package dashboard

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	terminalSessionTTL     = 30 * time.Second
	maxTerminalSessions    = 4
	maxTerminalMessageSize = 64 * 1024
	terminalProtocolPrefix = "otherhost-terminal."
)

type pendingTerminalSession struct {
	protocol    string
	projectPath string
	size        TerminalSize
	expiresAt   time.Time
}

func (handler *Handler) createTerminal(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(response, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if request.Header.Get("X-Otherhost-Token") != handler.token || !sameOrigin(request) {
		http.Error(response, "Action not authorized", http.StatusForbidden)
		return
	}
	if handler.terminal == nil {
		http.Error(response, "Terminal access is unavailable in demo mode", http.StatusUnprocessableEntity)
		return
	}
	if !handler.clientConnected() {
		http.Error(response, "Reconnect to the remote host before opening a terminal", http.StatusConflict)
		return
	}

	request.Body = http.MaxBytesReader(response, request.Body, 4096)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var payload struct {
		Path    string `json:"path"`
		Columns int    `json:"columns"`
		Rows    int    `json:"rows"`
	}
	if err := decoder.Decode(&payload); err != nil {
		http.Error(response, "Invalid terminal request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(response, "Invalid terminal request", http.StatusBadRequest)
		return
	}
	size, err := normalizeTerminalSize(payload.Columns, payload.Rows)
	if err != nil {
		http.Error(response, err.Error(), http.StatusBadRequest)
		return
	}

	handler.mutex.Lock()
	defer handler.mutex.Unlock()
	handler.removeExpiredTerminalSessionsLocked(time.Now())
	if handler.closing {
		http.Error(response, "The dashboard is shutting down", http.StatusServiceUnavailable)
		return
	}
	if payload.Path != "" {
		if _, allowed := handler.projects[payload.Path]; !allowed {
			http.Error(response, "Project is not available", http.StatusNotFound)
			return
		}
	}
	if len(handler.terminalSessions)+len(handler.activeTerminals) >= maxTerminalSessions {
		http.Error(response, "Too many terminal sessions are open", http.StatusTooManyRequests)
		return
	}

	id, err := randomToken(18)
	if err != nil {
		http.Error(response, "Could not create the terminal session", http.StatusInternalServerError)
		return
	}
	secret, err := randomToken(32)
	if err != nil {
		http.Error(response, "Could not create the terminal session", http.StatusInternalServerError)
		return
	}
	protocol := terminalProtocolPrefix + secret
	handler.terminalSessions[id] = pendingTerminalSession{
		protocol: protocol, projectPath: payload.Path, size: size, expiresAt: time.Now().Add(terminalSessionTTL),
	}

	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(response).Encode(struct {
		SocketPath string `json:"socketPath"`
		Protocol   string `json:"protocol"`
	}{SocketPath: "/api/terminals/" + id + "/socket", Protocol: protocol})
}

func (handler *Handler) serveTerminalSocket(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(response, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, ok := terminalSessionID(request.URL.Path)
	if !ok || !terminalWebSocketOriginAllowed(request) {
		http.Error(response, "Terminal session is not authorized", http.StatusForbidden)
		return
	}

	protocols := websocket.Subprotocols(request)
	handler.mutex.Lock()
	handler.removeExpiredTerminalSessionsLocked(time.Now())
	session, exists := handler.terminalSessions[id]
	if handler.closing {
		exists = false
	}
	if exists && !containsSecretProtocol(protocols, session.protocol) {
		exists = false
	}
	if exists {
		delete(handler.terminalSessions, id)
		handler.activeTerminals[id] = nil
	}
	handler.mutex.Unlock()
	if !exists {
		http.Error(response, "Terminal session is not authorized", http.StatusForbidden)
		return
	}

	upgrader := websocket.Upgrader{
		Subprotocols: []string{session.protocol},
		CheckOrigin: func(upgradeRequest *http.Request) bool {
			return terminalWebSocketOriginAllowed(upgradeRequest)
		},
	}
	connection, err := upgrader.Upgrade(response, request, nil)
	if err != nil {
		handler.removeActiveTerminal(id)
		return
	}
	defer connection.Close()
	connection.SetReadLimit(maxTerminalMessageSize)

	terminal, err := handler.terminal.StartTerminal(session.projectPath, session.size)
	if err != nil {
		handler.removeActiveTerminal(id)
		_ = connection.WriteMessage(websocket.BinaryMessage, []byte("\r\nOtherhost: "+err.Error()+"\r\n"))
		_ = connection.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Could not start the terminal"), time.Now().Add(time.Second))
		return
	}
	handler.mutex.Lock()
	if handler.closing {
		handler.mutex.Unlock()
		_ = terminal.Close()
		_ = connection.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "Dashboard is shutting down"), time.Now().Add(time.Second))
		return
	}
	handler.activeTerminals[id] = terminal
	handler.mutex.Unlock()
	defer func() {
		_ = terminal.Close()
		handler.removeActiveTerminal(id)
	}()

	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		buffer := make([]byte, 32*1024)
		for {
			count, readError := terminal.Read(buffer)
			if count > 0 {
				if writeError := connection.WriteMessage(websocket.BinaryMessage, buffer[:count]); writeError != nil {
					return
				}
			}
			if readError != nil {
				_ = connection.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Terminal closed"), time.Now().Add(time.Second))
				_ = connection.Close()
				return
			}
		}
	}()

	for {
		messageType, message, readError := connection.ReadMessage()
		if readError != nil {
			break
		}
		switch messageType {
		case websocket.BinaryMessage:
			if err := writeTerminalInput(terminal, message); err != nil {
				return
			}
		case websocket.TextMessage:
			if err := resizeTerminal(terminal, message); err != nil {
				_ = connection.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.ClosePolicyViolation, err.Error()), time.Now().Add(time.Second))
				return
			}
		default:
			return
		}
	}
	_ = terminal.Close()
	<-outputDone
}

func terminalSessionID(requestPath string) (string, bool) {
	const prefix = "/api/terminals/"
	const suffix = "/socket"
	if !strings.HasPrefix(requestPath, prefix) || !strings.HasSuffix(requestPath, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(requestPath, prefix), suffix)
	return id, id != "" && !strings.Contains(id, "/")
}

func terminalWebSocketOriginAllowed(request *http.Request) bool {
	return request.Header.Get("Origin") == "http://"+request.Host
}

func containsSecretProtocol(protocols []string, expected string) bool {
	for _, protocol := range protocols {
		if len(protocol) == len(expected) && subtle.ConstantTimeCompare([]byte(protocol), []byte(expected)) == 1 {
			return true
		}
	}
	return false
}

func (handler *Handler) removeExpiredTerminalSessionsLocked(now time.Time) {
	for id, session := range handler.terminalSessions {
		if !now.Before(session.expiresAt) {
			delete(handler.terminalSessions, id)
		}
	}
}

func (handler *Handler) removeActiveTerminal(id string) {
	handler.mutex.Lock()
	delete(handler.activeTerminals, id)
	handler.mutex.Unlock()
}

func writeTerminalInput(terminal Terminal, message []byte) error {
	for len(message) > 0 {
		written, err := terminal.Write(message)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		message = message[written:]
	}
	return nil
}

func resizeTerminal(terminal Terminal, message []byte) error {
	decoder := json.NewDecoder(strings.NewReader(string(message)))
	decoder.DisallowUnknownFields()
	var payload struct {
		Type    string `json:"type"`
		Columns int    `json:"columns"`
		Rows    int    `json:"rows"`
	}
	if err := decoder.Decode(&payload); err != nil {
		return errors.New("invalid terminal message")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) || payload.Type != "resize" {
		return errors.New("invalid terminal message")
	}
	size, err := normalizeTerminalSize(payload.Columns, payload.Rows)
	if err != nil {
		return err
	}
	if err := terminal.Resize(size); err != nil {
		return fmt.Errorf("could not resize the terminal: %w", err)
	}
	return nil
}

func (handler *Handler) CloseTerminals() {
	handler.mutex.Lock()
	handler.closing = true
	terminals := handler.closeActiveTerminalsLocked()
	handler.mutex.Unlock()
	closeTerminals(terminals)
}

func (handler *Handler) CloseActiveTerminals() {
	handler.mutex.Lock()
	terminals := handler.closeActiveTerminalsLocked()
	handler.mutex.Unlock()
	closeTerminals(terminals)
}

func (handler *Handler) closeActiveTerminalsLocked() []Terminal {
	terminals := make([]Terminal, 0, len(handler.activeTerminals))
	for id, terminal := range handler.activeTerminals {
		if terminal != nil {
			terminals = append(terminals, terminal)
		}
		delete(handler.activeTerminals, id)
	}
	handler.terminalSessions = make(map[string]pendingTerminalSession)
	return terminals
}

func closeTerminals(terminals []Terminal) {
	for _, terminal := range terminals {
		_ = terminal.Close()
	}
}
