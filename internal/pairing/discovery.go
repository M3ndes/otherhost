package pairing

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Device struct {
	Instance string
	Name     string
	Address  string
	Port     int
}

const (
	multicastSearchDuration = 1250 * time.Millisecond
	directProbeTimeout      = 500 * time.Millisecond
	maxSubnetCandidates     = 1024
	maxConcurrentProbes     = 64
)

func advertise(ctx context.Context, multicastAddress, instance, name string, port int, logger DiagnosticLog) error {
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
	logDiagnostic(logger, "multicast listener active on %s across %d interface(s)", multicastAddress, len(connections))
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

// DiscoverDevices first uses multicast and then automatically probes the
// client's local IPv4 subnet. The direct fallback keeps pairing usable when a
// VM or host firewall does not forward multicast into WSL.
func DiscoverDevices(ctx context.Context, multicastAddress string, pairPort int, logger DiagnosticLog) ([]Device, error) {
	if !validPort(pairPort) {
		return nil, errors.New("invalid pairing port")
	}
	logDiagnostic(logger, "multicast discovery sending to %s for %s", multicastAddress, multicastSearchDuration)
	multicastContext, cancel := context.WithTimeout(ctx, multicastSearchDuration)
	devices, multicastErr := Discover(multicastContext, multicastAddress)
	cancel()
	if len(devices) > 0 {
		logDiagnostic(logger, "multicast discovery found %d compatible devbox(es)", len(devices))
		return devices, multicastErr
	}
	logDiagnostic(logger, "multicast discovery received no compatible response")
	if ctx.Err() != nil {
		return devices, multicastErr
	}

	plan, err := localSubnetPlan()
	if err != nil {
		if multicastErr != nil {
			return nil, multicastErr
		}
		return nil, err
	}
	if len(plan.candidates) == 0 {
		logDiagnostic(logger, "direct discovery found no eligible local IPv4 subnet")
		return nil, nil
	}
	logDiagnostic(logger, "direct discovery probing %d address(es) on TCP %d in %s",
		len(plan.candidates), pairPort, strings.Join(plan.networks, ", "))
	directDevices := discoverAtAddresses(ctx, plan.candidates, pairPort, logger)
	if len(directDevices) > 0 {
		return directDevices, nil
	}
	return nil, nil
}

type subnetPlan struct {
	candidates []string
	networks   []string
}

func localSubnetPlan() (subnetPlan, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return subnetPlan{}, err
	}
	seen := make(map[string]struct{})
	seenNetworks := make(map[string]struct{})
	result := make([]string, 0, 256)
	networks := make([]string, 0, 2)
	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 ||
			networkInterface.Flags&net.FlagLoopback != 0 ||
			networkInterface.Flags&net.FlagPointToPoint != 0 ||
			networkInterface.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addresses, addressErr := networkInterface.Addrs()
		if addressErr != nil {
			continue
		}
		for _, address := range addresses {
			ipNetwork, ok := address.(*net.IPNet)
			if !ok {
				continue
			}
			localIP := ipNetwork.IP.To4()
			if localIP == nil || !isLocalNetworkIP(localIP) {
				continue
			}
			prefixLength, bits := ipNetwork.Mask.Size()
			if bits != 32 || prefixLength > 30 {
				continue
			}
			// Very large corporate and VPN routes are bounded to the local /24.
			// Normal /23-/30 LANs retain their real prefix.
			if prefixLength < 24 {
				prefixLength = 24
			}
			mask := net.CIDRMask(prefixLength, 32)
			network := binary.BigEndian.Uint32(localIP) & binary.BigEndian.Uint32(mask)
			networkBytes := make(net.IP, net.IPv4len)
			binary.BigEndian.PutUint32(networkBytes, network)
			networkLabel := (&net.IPNet{IP: networkBytes, Mask: mask}).String()
			if _, exists := seenNetworks[networkLabel]; !exists {
				seenNetworks[networkLabel] = struct{}{}
				networks = append(networks, networkLabel)
			}
			addressCount := uint32(1) << uint32(32-prefixLength)
			localValue := binary.BigEndian.Uint32(localIP)
			for offset := uint32(1); offset+1 < addressCount; offset++ {
				candidateValue := network + offset
				if candidateValue == localValue {
					continue
				}
				candidateBytes := make(net.IP, net.IPv4len)
				binary.BigEndian.PutUint32(candidateBytes, candidateValue)
				candidate := candidateBytes.String()
				if _, exists := seen[candidate]; exists {
					continue
				}
				seen[candidate] = struct{}{}
				result = append(result, candidate)
				if len(result) == maxSubnetCandidates {
					return subnetPlan{candidates: result, networks: networks}, nil
				}
			}
		}
	}
	return subnetPlan{candidates: result, networks: networks}, nil
}

type probeOutcome int

const (
	probeUnreachable probeOutcome = iota
	probeInvalidResponse
	probeCompatible
)

type addressProbeResult struct {
	device  Device
	outcome probeOutcome
}

func discoverAtAddresses(ctx context.Context, addresses []string, pairPort int, logger DiagnosticLog) []Device {
	if len(addresses) == 0 {
		return nil
	}
	nonce, err := randomHex(16)
	if err != nil {
		return nil
	}
	workerCount := maxConcurrentProbes
	if len(addresses) < workerCount {
		workerCount = len(addresses)
	}
	transport := &http.Transport{
		Proxy:             nil,
		DialContext:       (&net.Dialer{Timeout: directProbeTimeout}).DialContext,
		DisableKeepAlives: true,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: directProbeTimeout}
	jobs := make(chan string)
	results := make(chan addressProbeResult, workerCount)
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for address := range jobs {
				device, outcome := probeDevice(ctx, client, address, pairPort, nonce)
				select {
				case results <- addressProbeResult{device: device, outcome: outcome}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, address := range addresses {
			select {
			case jobs <- address:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	devices := make([]Device, 0)
	seen := make(map[string]struct{})
	attempted := 0
	responded := 0
	for result := range results {
		attempted++
		if result.outcome == probeInvalidResponse {
			responded++
		}
		if result.outcome != probeCompatible {
			continue
		}
		responded++
		device := result.device
		key := device.Instance + "@" + device.Address + ":" + strconv.Itoa(device.Port)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		devices = append(devices, device)
	}
	sort.Slice(devices, func(left, right int) bool {
		if devices[left].Name == devices[right].Name {
			return devices[left].Address < devices[right].Address
		}
		return devices[left].Name < devices[right].Name
	})
	logDiagnostic(logger, "direct discovery completed %d/%d probe(s): %d endpoint(s) responded, %d compatible devbox(es)",
		attempted, len(addresses), responded, len(devices))
	return devices
}

func probeDevice(ctx context.Context, client *http.Client, address string, pairPort int, nonce string) (Device, probeOutcome) {
	endpoint := "http://" + net.JoinHostPort(address, strconv.Itoa(pairPort)) +
		"/v1/discovery?nonce=" + url.QueryEscape(nonce)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Device{}, probeUnreachable
	}
	response, err := client.Do(request)
	if err != nil {
		return Device{}, probeUnreachable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Device{}, probeInvalidResponse
	}
	var advertisement DiscoveryResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, MaxMessageSize))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&advertisement) != nil ||
		advertisement.Magic != DiscoveryMagic ||
		advertisement.Version != ProtocolVersion ||
		advertisement.Nonce != nonce ||
		advertisement.Instance == "" || len(advertisement.Instance) > 64 ||
		advertisement.Port != pairPort {
		return Device{}, probeInvalidResponse
	}
	name, err := cleanDeviceName(advertisement.Name)
	if err != nil {
		return Device{}, probeInvalidResponse
	}
	return Device{
		Instance: advertisement.Instance,
		Name:     name,
		Address:  address,
		Port:     advertisement.Port,
	}, probeCompatible
}

func isLocalNetworkIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}
