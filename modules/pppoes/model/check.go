package pppoemodel

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// Refused is a request the operator can fix - a clash with an existing record, no
// free tunnel number - as opposed to a failure of the database. The API reports
// it as a 400 rather than a 500.
type Refused struct{ Msg string }

func (e *Refused) Error() string { return e.Msg }

func refuse(format string, a ...any) error { return &Refused{Msg: fmt.Sprintf(format, a...)} }

// maxTunnels is how many PPPoE servers there can be: one per pppac(4)
// interface, pppac0 to pppac7.
const maxTunnels = 8

// pppacNetmask is the netmask of a PPPoE server's own interface. npppd gives
// pppac(4) a single-address (/32) setup and hands each client a host route;
// with the parent LAN's /24 here instead, the pppac interface and the LAN
// interface would both claim the same subnet whenever netstart ran while npppd
// was up.
const pppacNetmask = "255.255.255.255"

// nextDevIndex returns the lowest pppac number not in use. Counting the
// existing records instead - what this used to do - hands out a number that is
// already taken as soon as an earlier record has been deleted (delete #0 of
// #0 and #1, add one, and it is numbered 1 again: two tunnels, one interface).
func nextDevIndex(used []int) (int, error) {
	taken := make(map[int]bool, len(used))
	for _, u := range used {
		taken[u] = true
	}
	for i := 0; i < maxTunnels; i++ {
		if !taken[i] {
			return i, nil
		}
	}
	return 0, refuse("all %d PPPoE servers (pppac0 to pppac%d) are in use", maxTunnels, maxTunnels-1)
}

// ipv4 parses a dotted-quad address, rejecting anything else (IPv6, hostnames,
// "1912.168.100.100").
func ipv4(s string) (net.IP, bool) {
	ip := net.ParseIP(strings.TrimSpace(s))
	if ip == nil || ip.To4() == nil || strings.Contains(s, ":") {
		return nil, false
	}
	return ip.To4(), true
}

func ipToUint(ip net.IP) uint32 {
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// checkPool requires pool to be exactly what npppd's pool-address takes here:
// one range "a.b.c.d-e.f.g.h" of IPv4 addresses, first not after last. A CIDR
// block is not accepted by npppd.conf (it is a syntax error), and a typo such as
// "1912.168.100.100" used to be stored and only failed when npppd was restarted.
// The server's own address must not be inside it, or a client would be handed
// the address the server itself answers on.
func checkPool(pool, serverIP string) error {
	first, last, ok := strings.Cut(pool, "-")
	if !ok || strings.ContainsAny(pool, " \t/,") {
		return errors.New("pool must be one address range, like 192.168.100.10-192.168.100.200")
	}
	a, okA := ipv4(first)
	b, okB := ipv4(last)
	if !okA || !okB {
		return fmt.Errorf("pool %q is not a range of two valid IPv4 addresses", pool)
	}
	if ipToUint(a) > ipToUint(b) {
		return fmt.Errorf("pool starts after it ends (%s is above %s)", first, last)
	}
	if s, ok := ipv4(serverIP); ok && ipToUint(s) >= ipToUint(a) && ipToUint(s) <= ipToUint(b) {
		return fmt.Errorf("pool %s contains the server's own address %s; start the pool above it", pool, serverIP)
	}
	return nil
}

// normalizeDNS turns what an operator typed - separated by spaces, commas or
// both - into the single-space list npppd.conf expects, and requires one or two
// IPv4 addresses. npppd refuses a comma-separated list, an empty list, and a
// third server (all three are syntax errors).
func normalizeDNS(s string) (string, error) {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
	if len(fields) == 0 {
		return "", errors.New("at least one DNS server is required")
	}
	if len(fields) > 2 {
		return "", fmt.Errorf("npppd takes at most two DNS servers, got %d", len(fields))
	}
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		ip, ok := ipv4(f)
		if !ok {
			return "", fmt.Errorf("DNS server %q is not a valid IPv4 address", f)
		}
		out = append(out, ip.String())
	}
	return strings.Join(out, " "), nil
}
