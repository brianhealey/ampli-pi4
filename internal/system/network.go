package system

import (
	"net"
	"path/filepath"
	"strings"
)

const sysClassNet = "/sys/class/net"

// ReadNetwork returns per-interface addressing and traffic counters.
//
// Loopback interfaces, and interfaces with neither an address nor any traffic,
// are omitted — a Raspberry Pi exposes a long tail of virtual links that add
// noise without telling an operator anything about reaching the device.
func ReadNetwork() []NetStats {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	out := make([]NetStats, 0, len(ifaces))
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		stats := NetStats{
			Name:      iface.Name,
			MAC:       iface.HardwareAddr.String(),
			Addresses: interfaceAddresses(iface),
			Up:        iface.Flags&net.FlagUp != 0,
			RxBytes:   readUint(filepath.Join(sysClassNet, iface.Name, "statistics", "rx_bytes")),
			TxBytes:   readUint(filepath.Join(sysClassNet, iface.Name, "statistics", "tx_bytes")),
		}
		if len(stats.Addresses) == 0 && stats.RxBytes == 0 && stats.TxBytes == 0 {
			continue
		}
		out = append(out, stats)
	}
	return out
}

// interfaceAddresses returns the unicast IPs assigned to an interface,
// excluding link-local addresses which are not useful for reaching the device.
func interfaceAddresses(iface net.Interface) []string {
	addrs, err := iface.Addrs()
	if err != nil {
		return []string{}
	}
	out := []string{}
	for _, a := range addrs {
		ip, _, err := net.ParseCIDR(a.String())
		if err != nil || ip.IsLinkLocalUnicast() {
			continue
		}
		out = append(out, strings.TrimSpace(ip.String()))
	}
	return out
}
