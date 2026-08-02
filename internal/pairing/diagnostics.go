package pairing

import "net"

// DiagnosticLog receives operational pairing details intended for a person
// troubleshooting local discovery. Callers decide how the message is rendered.
type DiagnosticLog func(format string, arguments ...any)

func logDiagnostic(logger DiagnosticLog, format string, arguments ...any) {
	if logger != nil {
		logger(format, arguments...)
	}
}

func remoteHost(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	return host
}
