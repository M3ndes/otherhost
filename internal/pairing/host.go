package pairing

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type HostOptions struct {
	Instance             string
	Name                 string
	Distro               string
	SSHUser              string
	SSHPort              int
	PairPort             int
	DiscoveryAddress     string
	Duration             time.Duration
	AuthorizedKeysFile   string
	SSHHostPublicKey     string
	Confirm              func(clientName, code string) bool
	InstallPublicKey     func(string) error
	ReadSSHHostPublicKey func() (string, error)
	DisableDiscovery     bool
}

type hostSession struct {
	id       string
	request  HelloRequest
	response HelloResponse
	keys     sessionKeys
	code     string
	decision chan bool
	expires  time.Time
}

type pairingHost struct {
	options  HostOptions
	instance string
	mu       sync.Mutex
	session  *hostSession
	done     chan error
}

func RunHost(ctx context.Context, options HostOptions) error {
	name, err := cleanDeviceName(options.Name)
	if err != nil {
		return err
	}
	options.Name = name
	if !validIdentifier(options.SSHUser) {
		return errors.New("invalid SSH user")
	}
	if !validPort(options.SSHPort) || !validPort(options.PairPort) {
		return errors.New("invalid network port")
	}
	if options.Duration < 10*time.Second || options.Duration > 10*time.Minute {
		return errors.New("pairing duration must be between 10 seconds and 10 minutes")
	}
	if options.Confirm == nil {
		return errors.New("host confirmation callback is required")
	}
	if err := configureHostCallbacks(&options); err != nil {
		return err
	}

	instance := options.Instance
	if instance == "" {
		instance, err = randomHex(16)
		if err != nil {
			return err
		}
	} else if len(instance) > 64 || strings.ContainsAny(instance, "\r\n") {
		return errors.New("invalid pairing instance")
	}
	host := &pairingHost{options: options, instance: instance, done: make(chan error, 1)}
	listener, err := net.Listen("tcp4", ":"+strconv.Itoa(options.PairPort))
	if err != nil {
		return fmt.Errorf("could not open pairing port %d: %w", options.PairPort, err)
	}
	defer listener.Close()

	router := http.NewServeMux()
	router.HandleFunc("/v1/hello", host.handleHello)
	router.HandleFunc("/v1/confirm", host.handleConfirm)
	router.HandleFunc("/v1/cancel", host.handleCancel)
	server := &http.Server{
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      options.Duration,
		IdleTimeout:       10 * time.Second,
		MaxHeaderBytes:    4096,
	}

	hostContext, cancel := context.WithTimeout(ctx, options.Duration)
	defer cancel()
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			host.finish(serveErr)
		}
	}()
	if !options.DisableDiscovery {
		go func() {
			if discoveryErr := advertise(hostContext, options.DiscoveryAddress, instance, options.Name, options.PairPort); discoveryErr != nil {
				host.finish(fmt.Errorf("local discovery failed: %w", discoveryErr))
			}
		}()
	}

	select {
	case result := <-host.done:
		cancel()
		shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownContext)
		return result
	case <-hostContext.Done():
		shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownContext)
		if errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		}
		return ErrPairingExpired
	}
}

func configureHostCallbacks(options *HostOptions) error {
	if options.InstallPublicKey == nil {
		switch {
		case options.AuthorizedKeysFile != "":
			options.InstallPublicKey = func(publicKey string) error {
				return installKeyInFile(options.AuthorizedKeysFile, publicKey)
			}
		case options.Distro != "":
			options.InstallPublicKey = func(publicKey string) error {
				return installKeyInWSL(options.Distro, publicKey)
			}
		default:
			return errors.New("host requires a WSL distribution or authorized_keys path")
		}
	}
	if options.ReadSSHHostPublicKey == nil {
		switch {
		case options.SSHHostPublicKey != "":
			normalized, err := normalizeEd25519PublicKey(options.SSHHostPublicKey)
			if err != nil {
				return fmt.Errorf("invalid SSH host key: %w", err)
			}
			options.ReadSSHHostPublicKey = func() (string, error) { return normalized, nil }
		case options.Distro != "":
			options.ReadSSHHostPublicKey = func() (string, error) { return readWSLHostKey(options.Distro) }
		default:
			return errors.New("host requires an SSH host public key source")
		}
	}
	return nil
}

func (host *pairingHost) handleHello(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || !localHTTPRequest(request) {
		http.Error(writer, "pairing is available only from the local network", http.StatusForbidden)
		return
	}
	var hello HelloRequest
	if err := decodeJSONBody(writer, request, &hello); err != nil {
		return
	}
	clientName, err := cleanDeviceName(hello.ClientName)
	if err != nil || hello.Version != ProtocolVersion || hello.Instance != host.instance {
		http.Error(writer, "invalid pairing hello", http.StatusBadRequest)
		return
	}
	hello.ClientName = clientName
	clientPublicKey, err := decode(hello.ClientPublicKey, 32)
	if err != nil {
		http.Error(writer, "invalid pairing hello", http.StatusBadRequest)
		return
	}
	if _, err := decode(hello.ClientNonce, 32); err != nil {
		http.Error(writer, "invalid pairing hello", http.StatusBadRequest)
		return
	}

	host.mu.Lock()
	if host.session != nil {
		host.mu.Unlock()
		http.Error(writer, "another device is already pairing", http.StatusConflict)
		return
	}
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		host.mu.Unlock()
		http.Error(writer, "could not create pairing session", http.StatusInternalServerError)
		return
	}
	hostNonce, err := randomBytes(32)
	if err != nil {
		host.mu.Unlock()
		http.Error(writer, "could not create pairing session", http.StatusInternalServerError)
		return
	}
	sessionID, err := randomHex(16)
	if err != nil {
		host.mu.Unlock()
		http.Error(writer, "could not create pairing session", http.StatusInternalServerError)
		return
	}
	expires := time.Now().Add(host.options.Duration)
	response := HelloResponse{
		SessionID: sessionID, HostName: host.options.Name,
		HostPublicKey: encode(privateKey.PublicKey().Bytes()), HostNonce: encode(hostNonce),
		ExpiresIn: int(host.options.Duration.Seconds()),
	}
	transcript := makeTranscript(host.instance, hello, response)
	keys, err := deriveSessionKeys(privateKey, clientPublicKey, transcript)
	if err != nil {
		host.mu.Unlock()
		http.Error(writer, "could not create pairing session", http.StatusBadRequest)
		return
	}
	session := &hostSession{
		id: sessionID, request: hello, response: response, keys: keys,
		code: comparisonCode(keys), decision: make(chan bool, 1), expires: expires,
	}
	host.session = session
	host.mu.Unlock()

	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(response); err != nil {
		host.finish(errors.New("client disconnected before numeric comparison"))
		return
	}
	go func() {
		approved := host.options.Confirm(session.request.ClientName, session.code)
		select {
		case session.decision <- approved:
		default:
		}
		if !approved {
			host.finish(ErrRejectedOnWindows)
		}
	}()
}

func (host *pairingHost) handleConfirm(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || !localHTTPRequest(request) {
		http.Error(writer, "pairing is available only from the local network", http.StatusForbidden)
		return
	}
	var confirmation ConfirmRequest
	if err := decodeJSONBody(writer, request, &confirmation); err != nil {
		return
	}
	session := host.currentSession(confirmation.SessionID)
	if session == nil || time.Now().After(session.expires) {
		http.Error(writer, "pairing session is not active", http.StatusGone)
		return
	}
	var payload PairPayload
	if err := openSealed(session.keys.clientToHost, session.keys.transcript, confirmation.Nonce, confirmation.Ciphertext, &payload); err != nil {
		http.Error(writer, "pairing confirmation could not be authenticated", http.StatusForbidden)
		return
	}
	publicKey, err := normalizeEd25519PublicKey(payload.SSHPublicKey)
	if err != nil {
		http.Error(writer, "invalid SSH public key", http.StatusBadRequest)
		return
	}

	select {
	case approved := <-session.decision:
		if !approved {
			http.Error(writer, "pairing was rejected on Windows", http.StatusForbidden)
			return
		}
	case <-request.Context().Done():
		return
	case <-time.After(time.Until(session.expires)):
		http.Error(writer, "pairing session expired", http.StatusGone)
		return
	}

	if err := host.options.InstallPublicKey(publicKey); err != nil {
		http.Error(writer, "could not authorize the Mac SSH key", http.StatusInternalServerError)
		host.finish(err)
		return
	}
	hostKey, err := host.options.ReadSSHHostPublicKey()
	if err != nil {
		http.Error(writer, "could not read the SSH host identity", http.StatusInternalServerError)
		host.finish(err)
		return
	}
	result := PairResult{
		DevboxName: host.options.Name,
		Host:       localHTTPAddress(request),
		SSHUser:    host.options.SSHUser,
		SSHPort:    host.options.SSHPort,
		SSHHostKey: hostKey,
	}
	nonce, ciphertext, err := seal(session.keys.hostToClient, session.keys.transcript, result)
	if err != nil {
		http.Error(writer, "could not finish pairing", http.StatusInternalServerError)
		host.finish(err)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(ConfirmResponse{Nonce: nonce, Ciphertext: ciphertext}); err != nil {
		host.finish(errors.New("client disconnected before pairing completed"))
		return
	}
	host.finish(nil)
}

func (host *pairingHost) handleCancel(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || !localHTTPRequest(request) {
		http.Error(writer, "pairing is available only from the local network", http.StatusForbidden)
		return
	}
	var cancellation ConfirmRequest
	if err := decodeJSONBody(writer, request, &cancellation); err != nil {
		return
	}
	session := host.currentSession(cancellation.SessionID)
	if session == nil {
		http.Error(writer, "pairing session is not active", http.StatusGone)
		return
	}
	var payload struct {
		Cancel bool `json:"cancel"`
	}
	if err := openSealed(session.keys.clientToHost, session.keys.transcript, cancellation.Nonce, cancellation.Ciphertext, &payload); err != nil || !payload.Cancel {
		http.Error(writer, "cancellation could not be authenticated", http.StatusForbidden)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
	host.finish(ErrRejectedOnMac)
}

func (host *pairingHost) currentSession(sessionID string) *hostSession {
	host.mu.Lock()
	defer host.mu.Unlock()
	if host.session == nil || host.session.id != sessionID {
		return nil
	}
	return host.session
}

func (host *pairingHost) finish(err error) {
	select {
	case host.done <- err:
	default:
	}
}

func localHTTPRequest(request *http.Request) bool {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return false
	}
	return isLocalNetworkIP(net.ParseIP(host))
}

func localHTTPAddress(request *http.Request) string {
	address, ok := request.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok {
		return ""
	}
	host, _, err := net.SplitHostPort(address.String())
	if err != nil {
		return ""
	}
	return host
}

func decodeJSONBody(writer http.ResponseWriter, request *http.Request, target any) error {
	request.Body = http.MaxBytesReader(writer, request.Body, MaxMessageSize)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		http.Error(writer, "invalid pairing message", http.StatusBadRequest)
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		http.Error(writer, "invalid pairing message", http.StatusBadRequest)
		return errors.New("pairing message contains trailing data")
	}
	return nil
}
