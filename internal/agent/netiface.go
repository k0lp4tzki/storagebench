package agent

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// managementIface determines the network interface that carries the connection
// to the controller at controllerURL. That interface is the "management" NIC and
// must never receive a fault, so a partition/loss fault can never cut the agent
// off from the controller.
//
// It resolves the controller host, then asks the kernel which local source
// address it would use to reach it (via a connect on a UDP socket, which does
// not send any packets), and finally maps that source IP to an interface.
//
// Returns an empty string (and no error) if the interface cannot be determined;
// callers treat "unknown management interface" conservatively.
func managementIface(controllerURL string) (string, error) {
	host, err := hostFromWSURL(controllerURL)
	if err != nil {
		return "", err
	}

	// Resolve to an IP we can route to. Prefer the first resolvable address.
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		// host may already be a literal IP.
		if ip := net.ParseIP(host); ip != nil {
			ips = []net.IP{ip}
		} else {
			return "", fmt.Errorf("resolve controller host %q: %w", host, err)
		}
	}

	srcIP, err := sourceIPFor(ips[0])
	if err != nil {
		return "", err
	}
	return ifaceForIP(srcIP)
}

// hostFromWSURL extracts the host (without port) from a ws:// or wss:// URL.
func hostFromWSURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse controller url: %w", err)
	}
	h := u.Hostname()
	if h == "" {
		return "", fmt.Errorf("controller url has no host: %q", raw)
	}
	return h, nil
}

// sourceIPFor returns the local source IP the kernel would use to reach dst. The
// UDP "connect" does not transmit; it only selects a route and source address.
func sourceIPFor(dst net.IP) (net.IP, error) {
	network := "udp4"
	if dst.To4() == nil {
		network = "udp6"
	}
	c, err := net.Dial(network, net.JoinHostPort(dst.String(), "9")) // discard port
	if err != nil {
		return nil, fmt.Errorf("select source route: %w", err)
	}
	defer c.Close()
	ua, ok := c.LocalAddr().(*net.UDPAddr)
	if !ok {
		return nil, fmt.Errorf("unexpected local addr type %T", c.LocalAddr())
	}
	return ua.IP, nil
}

// ifaceForIP maps a local IP address to the name of the interface that owns it.
func ifaceForIP(ip net.IP) (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("list interfaces: %w", err)
	}
	for _, ifc := range ifaces {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var aip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				aip = v.IP
			case *net.IPAddr:
				aip = v.IP
			}
			if aip != nil && aip.Equal(ip) {
				return ifc.Name, nil
			}
		}
	}
	return "", fmt.Errorf("no interface owns source ip %s", ip)
}

// normalizeIfaceList lowercases and de-duplicates an interface allowlist.
func normalizeIfaceList(in []string) map[string]bool {
	out := make(map[string]bool, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out[s] = true
		}
	}
	return out
}
