package pairing

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	ProtocolVersion         = 1
	DiscoveryMagic          = "devbox-bridge-discovery"
	DefaultDiscoveryAddress = "239.255.67.89:45870"
	DefaultPairPort         = 45871
	MaxMessageSize          = 16 * 1024
)

var (
	ErrPairingExpired    = errors.New("pairing mode expired before a device connected")
	ErrRejectedOnWindows = errors.New("pairing was rejected on Windows")
	ErrRejectedOnMac     = errors.New("pairing was rejected on the Mac")
)

type DiscoveryQuery struct {
	Magic   string `json:"magic"`
	Version int    `json:"version"`
	Nonce   string `json:"nonce"`
}

type DiscoveryResponse struct {
	Magic    string `json:"magic"`
	Version  int    `json:"version"`
	Nonce    string `json:"nonce"`
	Instance string `json:"instance"`
	Name     string `json:"name"`
	Port     int    `json:"port"`
}

type HelloRequest struct {
	Version         int    `json:"version"`
	Instance        string `json:"instance"`
	ClientName      string `json:"client_name"`
	ClientPublicKey string `json:"client_public_key"`
	ClientNonce     string `json:"client_nonce"`
}

type HelloResponse struct {
	SessionID     string `json:"session_id"`
	HostName      string `json:"host_name"`
	HostPublicKey string `json:"host_public_key"`
	HostNonce     string `json:"host_nonce"`
	ExpiresIn     int    `json:"expires_in_seconds"`
}

type ConfirmRequest struct {
	SessionID  string `json:"session_id"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type ConfirmResponse struct {
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type PairPayload struct {
	SSHPublicKey string `json:"ssh_public_key"`
}

type PairResult struct {
	DevboxName string `json:"devbox_name"`
	Host       string `json:"host"`
	SSHUser    string `json:"ssh_user"`
	SSHPort    int    `json:"ssh_port"`
	SSHHostKey string `json:"ssh_host_key"`
}

type sessionKeys struct {
	clientToHost []byte
	hostToClient []byte
	sas          []byte
	transcript   []byte
}

func randomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return nil, err
	}
	return value, nil
}

func randomHex(size int) (string, error) {
	value, err := randomBytes(size)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func encode(value []byte) string {
	return base64.RawStdEncoding.EncodeToString(value)
}

func decode(value string, expectedSize int) ([]byte, error) {
	decoded, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		return nil, errors.New("invalid base64 value")
	}
	if expectedSize > 0 && len(decoded) != expectedSize {
		return nil, fmt.Errorf("invalid value length: got %d, expected %d", len(decoded), expectedSize)
	}
	return decoded, nil
}

func writeField(buffer *bytes.Buffer, value string) {
	_ = binary.Write(buffer, binary.BigEndian, uint32(len(value)))
	buffer.WriteString(value)
}

func makeTranscript(instance string, request HelloRequest, response HelloResponse) []byte {
	var buffer bytes.Buffer
	writeField(&buffer, "devbox-bridge-pairing-v1")
	writeField(&buffer, instance)
	writeField(&buffer, request.ClientName)
	writeField(&buffer, response.HostName)
	writeField(&buffer, request.ClientPublicKey)
	writeField(&buffer, response.HostPublicKey)
	writeField(&buffer, request.ClientNonce)
	writeField(&buffer, response.HostNonce)
	writeField(&buffer, response.SessionID)
	writeField(&buffer, fmt.Sprintf("%d", response.ExpiresIn))
	return buffer.Bytes()
}

func hkdfExpand(secret, salt []byte, label string, length int) []byte {
	extract := hmac.New(sha256.New, salt)
	extract.Write(secret)
	prk := extract.Sum(nil)

	result := make([]byte, 0, length)
	var previous []byte
	for counter := byte(1); len(result) < length; counter++ {
		expand := hmac.New(sha256.New, prk)
		expand.Write(previous)
		expand.Write([]byte(label))
		expand.Write([]byte{counter})
		previous = expand.Sum(nil)
		result = append(result, previous...)
	}
	return result[:length]
}

func deriveSessionKeys(privateKey *ecdh.PrivateKey, peerPublicKey []byte, transcript []byte) (sessionKeys, error) {
	peer, err := ecdh.X25519().NewPublicKey(peerPublicKey)
	if err != nil {
		return sessionKeys{}, errors.New("invalid ephemeral public key")
	}
	shared, err := privateKey.ECDH(peer)
	if err != nil {
		return sessionKeys{}, errors.New("could not derive the shared pairing secret")
	}
	transcriptHash := sha256.Sum256(transcript)
	return sessionKeys{
		clientToHost: hkdfExpand(shared, transcriptHash[:], "devbox-bridge/client-to-host/v1", 32),
		hostToClient: hkdfExpand(shared, transcriptHash[:], "devbox-bridge/host-to-client/v1", 32),
		sas:          hkdfExpand(shared, transcriptHash[:], "devbox-bridge/numeric-comparison/v1", 32),
		transcript:   transcriptHash[:],
	}, nil
}

func comparisonCode(keys sessionKeys) string {
	mac := hmac.New(sha256.New, keys.sas)
	mac.Write(keys.transcript)
	value := binary.BigEndian.Uint32(mac.Sum(nil)[:4]) % 1000000
	return fmt.Sprintf("%06d", value)
}

func seal(key, associatedData []byte, value any) (nonce string, ciphertext string, err error) {
	plaintext, err := json.Marshal(value)
	if err != nil {
		return "", "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}
	nonceBytes, err := randomBytes(aead.NonceSize())
	if err != nil {
		return "", "", err
	}
	sealed := aead.Seal(nil, nonceBytes, plaintext, associatedData)
	return encode(nonceBytes), encode(sealed), nil
}

func openSealed(key, associatedData []byte, nonce, ciphertext string, target any) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonceBytes, err := decode(nonce, aead.NonceSize())
	if err != nil {
		return err
	}
	ciphertextBytes, err := decode(ciphertext, 0)
	if err != nil {
		return err
	}
	plaintext, err := aead.Open(nil, nonceBytes, ciphertextBytes, associatedData)
	if err != nil {
		return errors.New("encrypted pairing message could not be authenticated")
	}
	if err := json.Unmarshal(plaintext, target); err != nil {
		return errors.New("encrypted pairing message is invalid")
	}
	return nil
}

func cleanDeviceName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return "", errors.New("device name must contain between 1 and 64 characters")
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') &&
			!(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') &&
			!strings.ContainsRune("._ -", character) {
			return "", errors.New("device name contains unsupported characters")
		}
	}
	return value, nil
}

func validIdentifier(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') &&
			!(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') &&
			!strings.ContainsRune("._-", character) {
			return false
		}
	}
	return true
}

func validPort(port int) bool {
	return port > 0 && port <= 65535
}
