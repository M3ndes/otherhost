package pairing

import (
	"bytes"
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
	"time"
)

type ClientOptions struct {
	Device       Device
	ClientName   string
	SSHPublicKey string
	Confirm      func(hostName, code string) bool
}

type ClientResult struct {
	PairResult
}

func Pair(ctx context.Context, options ClientOptions) (ClientResult, error) {
	clientName, err := cleanDeviceName(options.ClientName)
	if err != nil {
		return ClientResult{}, err
	}
	hostName, err := cleanDeviceName(options.Device.Name)
	if err != nil {
		return ClientResult{}, err
	}
	if options.Device.Instance == "" || net.ParseIP(options.Device.Address) == nil || !validPort(options.Device.Port) {
		return ClientResult{}, errors.New("invalid discovered device")
	}
	if options.Confirm == nil {
		return ClientResult{}, errors.New("client confirmation callback is required")
	}
	publicKey, err := normalizeEd25519PublicKey(options.SSHPublicKey)
	if err != nil {
		return ClientResult{}, err
	}

	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return ClientResult{}, err
	}
	clientNonce, err := randomBytes(32)
	if err != nil {
		return ClientResult{}, err
	}
	hello := HelloRequest{
		Version: ProtocolVersion, Instance: options.Device.Instance, ClientName: clientName,
		ClientPublicKey: encode(privateKey.PublicKey().Bytes()), ClientNonce: encode(clientNonce),
	}
	baseURL := "http://" + net.JoinHostPort(options.Device.Address, strconv.Itoa(options.Device.Port))
	httpClient := &http.Client{Timeout: 2 * time.Minute}
	var response HelloResponse
	if err := postJSON(ctx, httpClient, baseURL+"/v1/hello", hello, &response); err != nil {
		return ClientResult{}, err
	}
	responseName, err := cleanDeviceName(response.HostName)
	if err != nil || responseName != hostName {
		return ClientResult{}, errors.New("host identity changed during discovery")
	}
	if response.SessionID == "" || len(response.SessionID) > 64 {
		return ClientResult{}, errors.New("host returned an invalid pairing session")
	}
	if response.ExpiresIn < 10 || response.ExpiresIn > 600 {
		return ClientResult{}, errors.New("host returned an invalid pairing expiry")
	}
	hostPublicKey, err := decode(response.HostPublicKey, 32)
	if err != nil {
		return ClientResult{}, errors.New("host returned an invalid ephemeral key")
	}
	if _, err := decode(response.HostNonce, 32); err != nil {
		return ClientResult{}, errors.New("host returned an invalid nonce")
	}
	transcript := makeTranscript(options.Device.Instance, hello, response)
	keys, err := deriveSessionKeys(privateKey, hostPublicKey, transcript)
	if err != nil {
		return ClientResult{}, err
	}
	code := comparisonCode(keys)
	if !options.Confirm(response.HostName, code) {
		nonce, ciphertext, sealErr := seal(keys.clientToHost, keys.transcript, struct {
			Cancel bool `json:"cancel"`
		}{Cancel: true})
		if sealErr == nil {
			_ = postJSON(ctx, httpClient, baseURL+"/v1/cancel", ConfirmRequest{
				SessionID: response.SessionID, Nonce: nonce, Ciphertext: ciphertext,
			}, nil)
		}
		return ClientResult{}, ErrRejectedOnMac
	}

	nonce, ciphertext, err := seal(keys.clientToHost, keys.transcript, PairPayload{SSHPublicKey: publicKey})
	if err != nil {
		return ClientResult{}, err
	}
	var confirmation ConfirmResponse
	if err := postJSON(ctx, httpClient, baseURL+"/v1/confirm", ConfirmRequest{
		SessionID: response.SessionID, Nonce: nonce, Ciphertext: ciphertext,
	}, &confirmation); err != nil {
		return ClientResult{}, err
	}
	var result PairResult
	if err := openSealed(keys.hostToClient, keys.transcript, confirmation.Nonce, confirmation.Ciphertext, &result); err != nil {
		return ClientResult{}, err
	}
	if _, err := cleanDeviceName(result.OtherhostName); err != nil || result.OtherhostName != response.HostName {
		return ClientResult{}, errors.New("paired host returned an invalid name")
	}
	if net.ParseIP(result.Host) == nil || !isLocalNetworkIP(net.ParseIP(result.Host)) {
		return ClientResult{}, errors.New("paired host returned an invalid local address")
	}
	if !validIdentifier(result.SSHUser) {
		return ClientResult{}, errors.New("paired host returned an invalid SSH user")
	}
	if !validPort(result.SSHPort) {
		return ClientResult{}, errors.New("paired host returned an invalid SSH port")
	}
	hostKey, err := normalizeEd25519PublicKey(result.SSHHostKey)
	if err != nil {
		return ClientResult{}, fmt.Errorf("paired host returned an invalid SSH host key: %w", err)
	}
	result.SSHHostKey = hostKey
	return ClientResult{PairResult: result}, nil
}

func postJSON(ctx context.Context, client *http.Client, url string, requestValue any, responseValue any) error {
	body, err := json.Marshal(requestValue)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("could not contact the Windows pairing host: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		messageText := strings.TrimSpace(string(message))
		if messageText == "" {
			messageText = response.Status
		}
		return fmt.Errorf("pairing host rejected the request: %s", messageText)
	}
	if responseValue == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, MaxMessageSize))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(responseValue); err != nil {
		return errors.New("pairing host returned an invalid response")
	}
	return nil
}
