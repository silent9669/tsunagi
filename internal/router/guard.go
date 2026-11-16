package router

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"
)

var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

func blockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || cgnat.Contains(ip)
}

// guardedTransport refuses connections to loopback, private, link-local and
// carrier-grade NAT addresses. It checks the address actually dialed, after
// DNS, so a hostname that resolves inward is caught. With a proxy configured in
// the environment the target address is not visible here, so the guard is off.
func guardedTransport(base *http.Transport) *http.Transport {
	t := base.Clone()
	for _, v := range []string{"HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"} {
		if os.Getenv(v) != "" {
			return t
		}
	}
	d := &net.Dialer{Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip := net.ParseIP(host); ip == nil || blockedIP(ip) {
			return fmt.Errorf("address %s is not allowed for a catalogue provider", host)
		}
		return nil
	}}
	t.DialContext = d.DialContext
	return t
}
