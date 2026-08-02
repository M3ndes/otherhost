package dashboard

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed assets/*
var embeddedAssets embed.FS

type ProjectLauncher interface {
	OpenProject(sshAlias, projectPath string) error
}

type ProjectDeleter interface {
	DeleteProject(projectPath string) error
}

type VSCodeLauncher struct {
	config *Config
}

func NewVSCodeLauncher(config Config) VSCodeLauncher {
	return VSCodeLauncher{config: &config}
}

func (launcher VSCodeLauncher) OpenProject(sshAlias, projectPath string) error {
	codePath, err := exec.LookPath("code")
	if err != nil {
		return errors.New("VS Code command-line tools are not installed")
	}
	if launcher.config != nil {
		sshPath, err := exec.LookPath("ssh")
		if err != nil {
			return errors.New("OpenSSH command-line tools are not installed")
		}
		resolvedConfig, err := exec.Command(sshPath, "-G", "--", sshAlias).Output()
		if err != nil || !sshAliasMatchesConfig(string(resolvedConfig), *launcher.config) {
			return fmt.Errorf("VS Code SSH alias %q is not configured; run otherhost ssh-config --apply and try again", sshAlias)
		}
	}
	remoteURI := url.URL{
		Scheme: "vscode-remote",
		Host:   "ssh-remote+" + sshAlias,
		Path:   projectPath,
	}
	if err := exec.Command(codePath, "--new-window", "--folder-uri", remoteURI.String()).Start(); err != nil {
		return fmt.Errorf("could not open VS Code: %w", err)
	}
	return nil
}

func sshAliasMatchesConfig(resolved string, config Config) bool {
	values := make(map[string][]string)
	for _, line := range strings.Split(resolved, "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), " ", 2)
		if len(fields) != 2 {
			continue
		}
		key := strings.ToLower(fields[0])
		values[key] = append(values[key], strings.Trim(strings.TrimSpace(fields[1]), `"`))
	}
	matches := func(key, expected string, foldCase bool) bool {
		for _, actual := range values[key] {
			if actual == expected || (foldCase && strings.EqualFold(actual, expected)) {
				return true
			}
		}
		return false
	}
	return matches("hostname", config.Host, true) &&
		matches("user", config.SSHUser, false) &&
		matches("port", strconv.Itoa(config.SSHPort), false) &&
		matches("identityfile", config.IdentityFile, false) &&
		(config.KnownHostsFile == "" || matches("userknownhostsfile", config.KnownHostsFile, false))
}

type Handler struct {
	collector Collector
	launcher  ProjectLauncher
	terminal  TerminalLauncher
	deleter   ProjectDeleter
	sshAlias  string
	token     string
	assets    http.Handler

	mutex            sync.RWMutex
	projects         map[string]Project
	deletingProjects map[string]struct{}
	terminalSessions map[string]pendingTerminalSession
	activeTerminals  map[string]Terminal
	closing          bool
}

func NewHandler(collector Collector, launcher ProjectLauncher, sshAlias string) (*Handler, error) {
	return NewHandlerWithTerminal(collector, launcher, nil, sshAlias)
}

func NewHandlerWithTerminal(collector Collector, launcher ProjectLauncher, terminal TerminalLauncher, sshAlias string) (*Handler, error) {
	return NewHandlerWithServices(collector, launcher, terminal, nil, sshAlias)
}

func NewHandlerWithServices(collector Collector, launcher ProjectLauncher, terminal TerminalLauncher, deleter ProjectDeleter, sshAlias string) (*Handler, error) {
	assetRoot, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		return nil, err
	}
	token, err := randomToken(32)
	if err != nil {
		return nil, fmt.Errorf("could not create dashboard session: %w", err)
	}
	return &Handler{
		collector:        collector,
		launcher:         launcher,
		terminal:         terminal,
		deleter:          deleter,
		sshAlias:         sshAlias,
		token:            token,
		assets:           http.FileServer(http.FS(assetRoot)),
		projects:         make(map[string]Project),
		deletingProjects: make(map[string]struct{}),
		terminalSessions: make(map[string]pendingTerminalSession),
		activeTerminals:  make(map[string]Terminal),
	}, nil
}

func randomToken(length int) (string, error) {
	tokenBytes := make([]byte, length)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(tokenBytes), nil
}

func (handler *Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	setSecurityHeaders(response)
	if !allowedRequestHost(request.Host) {
		http.Error(response, "Request host is not allowed", http.StatusMisdirectedRequest)
		return
	}
	switch request.URL.Path {
	case "/api/snapshot":
		handler.serveSnapshot(response, request)
	case "/api/projects/open":
		handler.openProject(response, request)
	case "/api/projects/delete":
		handler.deleteProject(response, request)
	case "/api/terminals":
		handler.createTerminal(response, request)
	default:
		if strings.HasPrefix(request.URL.Path, "/api/terminals/") {
			handler.serveTerminalSocket(response, request)
			return
		}
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			http.Error(response, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handler.assets.ServeHTTP(response, request)
	}
}

func allowedRequestHost(value string) bool {
	host := value
	if parsedHost, _, err := net.SplitHostPort(value); err == nil {
		host = parsedHost
	}
	return host == "127.0.0.1" || host == "localhost"
}

func (handler *Handler) serveSnapshot(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(response, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snapshot := handler.collector.Collect()
	handler.mutex.Lock()
	handler.projects = make(map[string]Project, len(snapshot.Projects))
	visibleProjects := make([]Project, 0, len(snapshot.Projects))
	for _, project := range snapshot.Projects {
		if _, deleting := handler.deletingProjects[project.Path]; !deleting {
			handler.projects[project.Path] = project
			visibleProjects = append(visibleProjects, project)
		}
	}
	snapshot.Projects = visibleProjects
	handler.mutex.Unlock()

	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(response).Encode(struct {
		Snapshot
		ActionToken            string `json:"actionToken"`
		ProjectDeletionEnabled bool   `json:"projectDeletionEnabled"`
	}{Snapshot: snapshot, ActionToken: handler.token, ProjectDeletionEnabled: handler.deleter != nil}); err != nil {
		http.Error(response, "Could not encode the dashboard response", http.StatusInternalServerError)
	}
}

func (handler *Handler) deleteProject(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(response, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if request.Header.Get("X-Otherhost-Token") != handler.token || !sameOrigin(request) {
		http.Error(response, "Action not authorized", http.StatusForbidden)
		return
	}
	if handler.deleter == nil {
		http.Error(response, "Project deletion is unavailable", http.StatusNotImplemented)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 4096)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var payload struct {
		Path         string `json:"path"`
		Confirmation string `json:"confirmation"`
	}
	if err := decoder.Decode(&payload); err != nil {
		http.Error(response, "Invalid project deletion request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(response, "Invalid project deletion request", http.StatusBadRequest)
		return
	}

	handler.mutex.Lock()
	if _, deleting := handler.deletingProjects[payload.Path]; deleting {
		handler.mutex.Unlock()
		http.Error(response, "Project deletion is already in progress", http.StatusConflict)
		return
	}
	project, allowed := handler.projects[payload.Path]
	if !allowed {
		handler.mutex.Unlock()
		http.Error(response, "Project is not available", http.StatusNotFound)
		return
	}
	if payload.Confirmation != project.Name {
		handler.mutex.Unlock()
		http.Error(response, "Project name confirmation does not match", http.StatusBadRequest)
		return
	}
	handler.deletingProjects[payload.Path] = struct{}{}
	delete(handler.projects, payload.Path)
	handler.mutex.Unlock()

	if err := handler.deleter.DeleteProject(payload.Path); err != nil {
		handler.mutex.Lock()
		delete(handler.deletingProjects, payload.Path)
		handler.projects[payload.Path] = project
		handler.mutex.Unlock()
		http.Error(response, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	handler.mutex.Lock()
	delete(handler.deletingProjects, payload.Path)
	handler.mutex.Unlock()
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = response.Write([]byte(`{"status":"deleted"}`))
}

func (handler *Handler) openProject(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(response, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if request.Header.Get("X-Otherhost-Token") != handler.token || !sameOrigin(request) {
		http.Error(response, "Action not authorized", http.StatusForbidden)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 4096)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var payload struct {
		Path string `json:"path"`
	}
	if err := decoder.Decode(&payload); err != nil {
		http.Error(response, "Invalid project request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(response, "Invalid project request", http.StatusBadRequest)
		return
	}
	handler.mutex.RLock()
	_, allowed := handler.projects[payload.Path]
	handler.mutex.RUnlock()
	if !allowed {
		http.Error(response, "Project is not available", http.StatusNotFound)
		return
	}
	if err := handler.launcher.OpenProject(handler.sshAlias, payload.Path); err != nil {
		http.Error(response, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(http.StatusAccepted)
	_, _ = response.Write([]byte(`{"status":"opening"}`))
}

func sameOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return true
	}
	return origin == "http://"+request.Host
}

func setSecurityHeaders(response http.ResponseWriter) {
	response.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	response.Header().Set("Referrer-Policy", "no-referrer")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.Header().Set("X-Frame-Options", "DENY")
}

func RunServer(ctx context.Context, collector Collector, launcher ProjectLauncher, terminal TerminalLauncher, deleter ProjectDeleter, sshAlias string, port int, openBrowser bool) error {
	if port < 0 || port > 65535 {
		return errors.New("dashboard port must be between 0 and 65535")
	}
	handler, err := NewHandlerWithServices(collector, launcher, terminal, deleter, sshAlias)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("could not start the local dashboard: %w", err)
	}
	dashboardURL := "http://" + listener.Addr().String() + "/"
	fmt.Printf("Otherhost dashboard: %s\nPress Ctrl-C to stop.\n", dashboardURL)

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.Serve(listener)
	}()
	if openBrowser {
		if err := OpenBrowser(dashboardURL); err != nil {
			fmt.Printf("Open the dashboard URL in your browser. (%s)\n", err)
		}
	}

	select {
	case <-ctx.Done():
		handler.CloseTerminals()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return server.Shutdown(shutdownContext)
	case err := <-serveErrors:
		handler.CloseTerminals()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
