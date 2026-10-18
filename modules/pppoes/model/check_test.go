package pppoemodel

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// The cases below are what `npppd -n` did with the same values on the gateway
// (OpenBSD 7.9): a typo'd range was "Invalid argument", a comma-separated or
// empty dns-servers and a third server were syntax errors, and a CIDR pool was a
// syntax error. Each one used to be accepted and stored, and only failed when
// npppd was restarted.

func TestCheckPool(t *testing.T) {
	good := []string{"192.168.100.10-192.168.100.200", "10.0.0.2-10.0.0.254", "192.168.200.10-192.168.200.10"}
	for _, p := range good {
		if err := checkPool(p, "192.168.100.2"); err != nil && !strings.Contains(err.Error(), "own address") {
			t.Errorf("checkPool(%q) = %v, want nil", p, err)
		}
	}
	if err := checkPool("192.168.100.10-192.168.100.200", "192.168.100.2"); err != nil {
		t.Errorf("a normal pool beside the server address was refused: %v", err)
	}
	bad := map[string]string{
		"192.168.100.10-1912.168.100.100": "the typo that was stored on the gateway",
		"192.168.100.0/25":                "CIDR is a syntax error in npppd.conf",
		"192.168.100.200-192.168.100.10":  "starts after it ends",
		"192.168.100.10":                  "not a range",
		"192.168.100.10 - 192.168.100.20": "spaces",
		"192.168.100.10,192.168.100.20":   "comma list",
		"a.b.c.d-e.f.g.h":                 "not addresses",
		"::1-::5":                         "IPv6",
		"":                                "empty",
		"192.168.100.10-192.168.100.20\n}\ninterface pppac9": "config injection",
	}
	for p, why := range bad {
		if err := checkPool(p, "192.168.100.2"); err == nil {
			t.Errorf("checkPool(%q) accepted (%s)", p, why)
		}
	}
}

func TestPoolMayNotContainTheServerAddress(t *testing.T) {
	if err := checkPool("192.168.100.1-192.168.100.254", "192.168.100.2"); err == nil || !strings.Contains(err.Error(), "own address") {
		t.Errorf("pool containing the server address: %v", err)
	}
	if err := checkPool("192.168.100.3-192.168.100.254", "192.168.100.2"); err != nil {
		t.Errorf("pool starting just above the server address was refused: %v", err)
	}
}

func TestNormalizeDNS(t *testing.T) {
	good := map[string]string{
		"9.9.9.9 4.2.2.2":     "9.9.9.9 4.2.2.2",
		"1.1.1.1,8.8.8.8":     "1.1.1.1 8.8.8.8", // the form says commas are fine; npppd says otherwise
		"1.1.1.1, 8.8.8.8":    "1.1.1.1 8.8.8.8",
		"  1.1.1.1   ":        "1.1.1.1",
		"8.8.8.8  ,  9.9.9.9": "8.8.8.8 9.9.9.9",
	}
	for in, want := range good {
		got, err := normalizeDNS(in)
		if err != nil || got != want {
			t.Errorf("normalizeDNS(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "   ", ",", "1.1.1.1 8.8.8.8 9.9.9.9", "dns.example.com", "1.1.1.256", "::1", "1.1.1.1;reboot", "1.1.1.1\n}"} {
		if got, err := normalizeDNS(in); err == nil {
			t.Errorf("normalizeDNS(%q) = %q, want an error", in, got)
		}
	}
}

// Deleting an earlier record must not make the next one reuse a taken number.
func TestNextDevIndexTakesTheLowestFreeNumber(t *testing.T) {
	cases := []struct {
		used []int
		want int
	}{
		{nil, 0},
		{[]int{0}, 1},
		{[]int{0, 1}, 2},
		{[]int{1}, 0},    // record 0 was deleted: the old "count the records" gave 1, already in use
		{[]int{0, 2}, 1}, // a gap in the middle
		{[]int{1, 2, 3}, 0},
	}
	for _, c := range cases {
		if got, err := nextDevIndex(c.used); err != nil || got != c.want {
			t.Errorf("nextDevIndex(%v) = %d, %v; want %d", c.used, got, err, c.want)
		}
	}
	if _, err := nextDevIndex([]int{0, 1, 2, 3, 4, 5, 6, 7}); err == nil {
		t.Error("a ninth server was allowed; there are only pppac0 to pppac7")
	}
}

func valid() *Pppoe {
	return &Pppoe{Name: "lan1ppp", Device: "em2", Ip: "192.168.100.2",
		PoolAddress: "192.168.100.10-192.168.100.200", DnsAddress: "9.9.9.9 4.2.2.2"}
}

func TestBindAcceptsAGoodRecordAndTidiesTheDNSList(t *testing.T) {
	p := valid()
	p.DnsAddress = "1.1.1.1,8.8.8.8"
	if err := p.Bind(&http.Request{}); err != nil {
		t.Fatalf("a valid record was refused: %v", err)
	}
	if p.DnsAddress != "1.1.1.1 8.8.8.8" {
		t.Errorf("dns = %q, want it rewritten the way npppd.conf needs it", p.DnsAddress)
	}
}

func TestBindRefusesWhatWouldBreakNpppd(t *testing.T) {
	cases := map[string]func(*Pppoe){
		"no name":             func(p *Pppoe) { p.Name = "" },
		"no device":           func(p *Pppoe) { p.Device = "" },
		"no ip":               func(p *Pppoe) { p.Ip = "" },
		"ip is IPv6":          func(p *Pppoe) { p.Ip = "fe80::1" },
		"typo in the pool":    func(p *Pppoe) { p.PoolAddress = "192.168.100.10-1912.168.100.100" },
		"CIDR pool":           func(p *Pppoe) { p.PoolAddress = "192.168.100.0/25" },
		"pool holds the ip":   func(p *Pppoe) { p.PoolAddress = "192.168.100.1-192.168.100.254" },
		"no dns":              func(p *Pppoe) { p.DnsAddress = "" },
		"three dns":           func(p *Pppoe) { p.DnsAddress = "1.1.1.1 8.8.8.8 9.9.9.9" },
		"name with a space":   func(p *Pppoe) { p.Name = "lan 1" },
		"name injects config": func(p *Pppoe) { p.Name = "x\"; reboot" },
		"index out of range":  func(p *Pppoe) { p.DevIndex = 9 },
	}
	for name, mutate := range cases {
		p := valid()
		mutate(p)
		if err := p.Bind(&http.Request{}); err == nil {
			t.Errorf("%s: record accepted, want it refused", name)
		}
	}
}

// What is written to npppd.conf for a record that passed Bind must itself be
// something the file format allows.
func TestAGoodRecordRendersAConfigWithNoStrayText(t *testing.T) {
	p := valid()
	p.DevIndex = 1
	if err := p.Bind(&http.Request{}); err != nil {
		t.Fatal(err)
	}
	conf := p.ToConf()
	for _, want := range []string{
		"tunnel PPPOE1 protocol pppoe {", "listen on interface em2",
		"pool-address 192.168.100.10-192.168.100.200", "dns-servers 9.9.9.9 4.2.2.2",
		"interface pppac1 address 192.168.100.2 ipcp IPCP1", "bind tunnel from PPPOE1 authenticated by LOCAL to pppac1",
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("config lacks %q:\n%s", want, conf)
		}
	}
}

// A request the operator can fix is reported as such, so the API answers 400
// rather than 500 "DB error".
func TestRefusalsAreTypedSoTheAPICanAnswer400(t *testing.T) {
	_, err := nextDevIndex([]int{0, 1, 2, 3, 4, 5, 6, 7})
	var refused *Refused
	if !errors.As(err, &refused) {
		t.Errorf("running out of tunnel numbers returned %T, want *Refused", err)
	}
}
