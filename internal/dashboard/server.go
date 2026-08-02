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
	"sync"
	"time"
)

//go:embed assets/*
var embeddedAssets embed.FS

type ProjectLauncher interface {
	OpenProject(sshAlias, projectPath string) error
}

type VSCodeLauncher struct{}

func (VSCodeLauncher) OpenProject(sshAlias, projectPath string) error {
	codePath, err := exec.LookPath("code")
	if err != nil {
		return errors.New("VS Code command-line tools are not installed")
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

type Handler struct {
	collector Collector
	launcher  ProjectLauncher
	sshAlias  string
	token     string
	assets    http.Handler

	mutex    sync.RWMutex
	projects map[string]struct{}
}

func NewHandler(collector Collector, launcher ProjectLauncher, sshAlias string) (*Handler, error) {
	assetRoot, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		return nil, err
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("could not create dashboard session: %w", err)
	}
	return &Handler{
		collector: collector,
		launcher:  launcher,
		sshAlias:  sshAlias,
		token:     base64.RawURLEncoding.EncodeToString(tokenBytes),
		assets:    http.FileServer(http.FS(assetRoot)),
		projects:  make(map[string]struct{}),
	}, nil
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
	default:
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
	handler.projects = make(map[string]struct{}, len(snapshot.Projects))
	for _, project := range snapshot.Projects {
		handler.projects[project.Path] = struct{}{}
	}
	handler.mutex.Unlock()

	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(response).Encode(struct {
		Snapshot
		ActionToken string `json:"actionToken"`
	}{Snapshot: snapshot, ActionToken: handler.token}); err != nil {
		http.Error(response, "Could not encode the dashboard response", http.StatusInternalServerError)
	}
}

func (handler *Handler) openProject(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(response, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if request.Header.Get("X-Devbox-Token") != handler.token || !sameOrigin(request) {
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

func RunServer(ctx context.Context, collector Collector, launcher ProjectLauncher, sshAlias string, port int, openBrowser bool) error {
	if port < 0 || port > 65535 {
		return errors.New("dashboard port must be between 0 and 65535")
	}
	handler, err := NewHandler(collector, launcher, sshAlias)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("could not start the local dashboard: %w", err)
	}
	dashboardURL := "http://" + listener.Addr().String() + "/"
	fmt.Printf("Devbox dashboard: %s\nPress Ctrl-C to stop.\n", dashboardURL)

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
		shutdownContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return server.Shutdown(shutdownContext)
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
