package pairing

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sort"
	"strconv"
	"time"
)

type Device struct {
	Instance string
	Name     string
	Address  string
	Port     int
}

func advertise(ctx context.Context, multicastAddress, instance, name string, port int) error {
	address, err := net.ResolveUDPAddr("udp4", multicastAddress)
	if err != nil {
		return err
	}
	interfaces, err := multicastInterfaces()
	if err != nil {
		return err
	}
	connections := make([]*net.UDPConn, 0, len(interfaces))
	for index := range interfaces {
		connection, listenErr := net.ListenMulticastUDP("udp4", &interfaces[index], address)
		if listenErr != nil {
			continue
		}
		_ = connection.SetReadBuffer(64 * 1024)
		connections = append(connections, connection)
	}
	if len(connections) == 0 {
		connection, listenErr := net.ListenMulticastUDP("udp4", nil, address)
		if listenErr != nil {
			return listenErr
		}
		connections = append(connections, connection)
	}
	defer func() {
		for _, connection := range connections {
			_ = connection.Close()
		}
	}()

	errorsFromListeners := make(chan error, len(connections))
	for _, connection := range connections {
		go func(connection *net.UDPConn) {
			errorsFromListeners <- serveAdvertisements(ctx, connection, instance, name, port)
		}(connection)
	}
	remaining := len(connections)
	var lastError error
	for remaining > 0 {
		select {
		case <-ctx.Done():
			return nil
		case listenerError := <-errorsFromListeners:
			remaining--
			if listenerError != nil {
				lastError = listenerError
			}
		}
	}
	return lastError
}

func serveAdvertisements(ctx context.Context, connection *net.UDPConn, instance, name string, port int) error {
	buffer := make([]byte, 2048)
	for {
		if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			return err
		}
		size, remote, err := connection.ReadFromUDP(buffer)
		if err != nil {
			if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
				select {
				case <-ctx.Done():
					return nil
				default:
					continue
				}
			}
			return err
		}
		if !isLocalNetworkIP(remote.IP) || size > MaxMessageSize {
			continue
		}
		var query DiscoveryQuery
		if json.Unmarshal(buffer[:size], &query) != nil ||
			query.Magic != DiscoveryMagic ||
			query.Version != ProtocolVersion ||
			query.Nonce == "" || len(query.Nonce) > 64 {
			continue
		}
		response, err := json.Marshal(DiscoveryResponse{
			Magic: DiscoveryMagic, Version: ProtocolVersion, Nonce: query.Nonce,
			Instance: instance, Name: name, Port: port,
		})
		if err != nil {
			continue
		}
		_, _ = connection.WriteToUDP(response, remote)
	}
}

func multicastInterfaces() ([]net.Interface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	result := make([]net.Interface, 0, len(interfaces))
	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 || networkInterface.Flags&net.FlagMulticast == 0 || networkInterface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, addressErr := networkInterface.Addrs()
		if addressErr != nil {
			continue
		}
		for _, address := range addresses {
			ip, _, parseErr := net.ParseCIDR(address.String())
			if parseErr == nil && ip.To4() != nil && isLocalNetworkIP(ip) {
				result = append(result, networkInterface)
				break
			}
		}
	}
	return result, nil
}

func Discover(ctx context.Context, multicastAddress string) ([]Device, error) {
	address, err := net.ResolveUDPAddr("udp4", multicastAddress)
	if err != nil {
		return nil, err
	}
	connection, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}
	defer connection.Close()

	nonce, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	query, err := json.Marshal(DiscoveryQuery{Magic: DiscoveryMagic, Version: ProtocolVersion, Nonce: nonce})
	if err != nil {
		return nil, err
	}

	devices := map[string]Device{}
	buffer := make([]byte, 2048)
	nextQuery := time.Time{}
	for {
		select {
		case <-ctx.Done():
			result := make([]Device, 0, len(devices))
			for _, device := range devices {
				result = append(result, device)
			}
			sort.Slice(result, func(left, right int) bool {
				if result[left].Name == result[right].Name {
					return result[left].Address < result[right].Address
				}
				return result[left].Name < result[right].Name
			})
			return result, nil
		default:
		}

		if time.Now().After(nextQuery) {
			if _, err := connection.WriteToUDP(query, address); err != nil {
				return nil, err
			}
			nextQuery = time.Now().Add(750 * time.Millisecond)
		}
		deadline := time.Now().Add(250 * time.Millisecond)
		if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
			deadline = contextDeadline
		}
		_ = connection.SetReadDeadline(deadline)
		size, remote, readErr := connection.ReadFromUDP(buffer)
		if readErr != nil {
			if timeout, ok := readErr.(net.Error); ok && timeout.Timeout() {
				continue
			}
			if errors.Is(readErr, net.ErrClosed) || ctx.Err() != nil {
				continue
			}
			return nil, readErr
		}
		if !isLocalNetworkIP(remote.IP) || size > MaxMessageSize {
			continue
		}
		var response DiscoveryResponse
		if json.Unmarshal(buffer[:size], &response) != nil ||
			response.Magic != DiscoveryMagic ||
			response.Version != ProtocolVersion ||
			response.Nonce != nonce ||
			response.Instance == "" ||
			!validPort(response.Port) {
			continue
		}
		name, err := cleanDeviceName(response.Name)
		if err != nil {
			continue
		}
		devices[response.Instance+"@"+remote.IP.String()+":"+strconv.Itoa(response.Port)] = Device{
			Instance: response.Instance,
			Name:     name,
			Address:  remote.IP.String(),
			Port:     response.Port,
		}
	}
}

func isLocalNetworkIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}
