// Package validate provides input-validation helpers used by module models'
// Bind() implementations.
//
// It intentionally depends on nothing but the standard library. The
// top-level utils package already imports modules/vouchers/model (for
// CheckExpired), so if model packages validated via utils instead of this
// package, modules/vouchers/model would import utils, which imports
// modules/vouchers/model - an import cycle. Keeping this package leaf-level
// lets every model package, including vouchers, import it safely.
package validate

import (
	"fmt"
	"math"
	"net"
	"regexp"
	"strings"
	"time"
)

// MaxStringLen is the default maximum length enforced on free-form string fields.
const MaxStringLen = 100

var (
	alnumRegex = regexp.MustCompile(`^[A-Za-z0-9]+$`)
	// alnumSpaceRegex is alnumRegex plus interior spaces, for name-like
	// fields (first/last name, location) that legitimately span words.
	alnumSpaceRegex = regexp.MustCompile(`^[A-Za-z0-9 ]+$`)
	// safeTokenRegex allows the punctuation commonly needed by pf.conf /
	// dhcpd.conf fragments (CIDR blocks, port ranges, comma-separated
	// lists) while rejecting quotes, semicolons, newlines, braces, '$' and
	// backticks that could inject extra directives into a generated config.
	safeTokenRegex = regexp.MustCompile(`^[A-Za-z0-9./:_,\* -]+$`)
	// safeTokenExtRegex is safeTokenRegex plus '(', ')', '<' and '>', for
	// fields that need parenthesized groups or comparison operators embedded
	// in generated config fragments.
	safeTokenExtRegex = regexp.MustCompile(`^[A-Za-z0-9./:_,\*()<> -]+$`)
	// safeTokenPortRegex is safeTokenRegex plus '!', '=', '<' and '>', for
	// pf.conf port-match expressions, which use those characters as unary
	// and binary operators (e.g. "!= 22", "2000:2004", "2000 >< 2004").
	safeTokenPortRegex = regexp.MustCompile(`^[A-Za-z0-9./:_,\*!=<> -]+$`)
)

func maxLenOrDefault(maxLen int) int {
	if maxLen <= 0 {
		return MaxStringLen
	}
	return maxLen
}

// Alnum requires value to be empty or strictly alphanumeric, up to maxLen
// characters (MaxStringLen if maxLen <= 0). Empty values are accepted here
// since this API's routes read a full struct from the request body for both
// create and update - callers that must reject a blank field should also
// call Required.
func Alnum(field, value string, maxLen int) error {
	if value == "" {
		return nil
	}
	maxLen = maxLenOrDefault(maxLen)
	if len(value) > maxLen {
		return fmt.Errorf("%s exceeds maximum length of %d characters", field, maxLen)
	}
	if !alnumRegex.MatchString(value) {
		return fmt.Errorf("%s must be alphanumeric", field)
	}
	return nil
}

// AlnumSpace requires value to be empty or alphanumeric with interior
// spaces allowed, up to maxLen characters (MaxStringLen if maxLen <= 0).
// Use this instead of Alnum for fields like names or locations that may
// contain multiple words.
func AlnumSpace(field, value string, maxLen int) error {
	if value == "" {
		return nil
	}
	maxLen = maxLenOrDefault(maxLen)
	if len(value) > maxLen {
		return fmt.Errorf("%s exceeds maximum length of %d characters", field, maxLen)
	}
	if !alnumSpaceRegex.MatchString(value) {
		return fmt.Errorf("%s must be alphanumeric", field)
	}
	return nil
}

// Required rejects a blank (or whitespace-only) value.
func Required(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	return nil
}

// MaxLen rejects a value longer than maxLen (MaxStringLen if maxLen <= 0),
// without restricting its character set. Use this instead of Alnum for
// fields that legitimately need punctuation but aren't validated by a more
// specific helper (e.g. passwords, which shouldn't be charset-restricted).
func MaxLen(field, value string, maxLen int) error {
	maxLen = maxLenOrDefault(maxLen)
	if len(value) > maxLen {
		return fmt.Errorf("%s exceeds maximum length of %d characters", field, maxLen)
	}
	return nil
}

// SafeToken allows letters, digits, and '.', ':', '/', '_', '-', ',', '*' and
// space, up to maxLen characters (MaxStringLen if maxLen <= 0). It's meant
// for fields that get embedded into generated config file fragments (pf
// rules, dhcpd ranges) and need punctuation Alnum would reject, but still
// must not carry characters that could inject extra config directives.
func SafeToken(field, value string, maxLen int) error {
	if value == "" {
		return nil
	}
	maxLen = maxLenOrDefault(maxLen)
	if len(value) > maxLen {
		return fmt.Errorf("%s exceeds maximum length of %d characters", field, maxLen)
	}
	if !safeTokenRegex.MatchString(value) {
		return fmt.Errorf("%s contains unsupported characters", field)
	}
	return nil
}

// SafeTokenExt is SafeToken plus '(', ')', '<' and '>', up to maxLen
// characters (MaxStringLen if maxLen <= 0). Use this instead of SafeToken
// for fields that need parenthesized groups or comparison operators (e.g.
// pf.conf match expressions) while still rejecting quotes, semicolons,
// braces, '$' and backticks.
func SafeTokenExt(field, value string, maxLen int) error {
	if value == "" {
		return nil
	}
	maxLen = maxLenOrDefault(maxLen)
	if len(value) > maxLen {
		return fmt.Errorf("%s exceeds maximum length of %d characters", field, maxLen)
	}
	if !safeTokenExtRegex.MatchString(value) {
		return fmt.Errorf("%s contains unsupported characters", field)
	}
	return nil
}

// SafeTokenPort is SafeToken plus '!', '=', '<' and '>', up to maxLen
// characters (MaxStringLen if maxLen <= 0). Use this for pf.conf port-match
// fields, which support unary operators (=, !=, <, <=, >, >=) and binary
// range operators (":", "><", "<>"), e.g. "2000:2004" or "2000 >< 2004".
func SafeTokenPort(field, value string, maxLen int) error {
	if value == "" {
		return nil
	}
	maxLen = maxLenOrDefault(maxLen)
	if len(value) > maxLen {
		return fmt.Errorf("%s exceeds maximum length of %d characters", field, maxLen)
	}
	if !safeTokenPortRegex.MatchString(value) {
		return fmt.Errorf("%s contains unsupported characters", field)
	}
	return nil
}

// IP requires value to be empty or a syntactically valid IPv4/IPv6 address.
// Dotted-decimal netmasks (e.g. "255.255.255.0") are accepted too, since
// they parse as valid IPv4 addresses.
func IP(field, value string) error {
	if value == "" {
		return nil
	}
	if net.ParseIP(value) == nil {
		return fmt.Errorf("%s is not a valid IP address", field)
	}
	return nil
}

// CIDR requires value to be empty or a syntactically valid CIDR block.
func CIDR(field, value string) error {
	if value == "" {
		return nil
	}
	if _, _, err := net.ParseCIDR(value); err != nil {
		return fmt.Errorf("%s is not a valid CIDR block", field)
	}
	return nil
}

// MAC requires value to be empty or a syntactically valid MAC address.
func MAC(field, value string) error {
	if value == "" {
		return nil
	}
	if _, err := net.ParseMAC(value); err != nil {
		return fmt.Errorf("%s is not a valid MAC address", field)
	}
	return nil
}

// IPList validates a comma and/or whitespace separated list of IP addresses.
func IPList(field, value string) error {
	if value == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	if len(parts) == 0 {
		return fmt.Errorf("%s is not a valid IP list", field)
	}
	for _, p := range parts {
		if net.ParseIP(p) == nil {
			return fmt.Errorf("%s contains an invalid IP address: %q", field, p)
		}
	}
	return nil
}

// IPRange validates a whitespace-separated "<start> <end>" IP pair, the
// format used by isc-dhcpd's "range" directive.
func IPRange(field, value string) error {
	if value == "" {
		return nil
	}
	parts := strings.Fields(value)
	if len(parts) != 2 {
		return fmt.Errorf("%s must be two IP addresses separated by a space", field)
	}
	for _, p := range parts {
		if net.ParseIP(p) == nil {
			return fmt.Errorf("%s contains an invalid IP address: %q", field, p)
		}
	}
	return nil
}

// Date requires value to be empty or parseable with the given time.Parse layout.
func Date(field, value, layout string) error {
	if value == "" {
		return nil
	}
	if _, err := time.Parse(layout, value); err != nil {
		return fmt.Errorf("%s is not a valid date (expected format %s)", field, layout)
	}
	return nil
}

// Int bounds-checks a signed integer field against the int32 range, which is
// generous enough for every numeric field in this API (speeds, counts,
// durations) while guarding downstream arithmetic (bandwidth math, duration
// calculations, pf.conf rule generation) against overflow.
func Int(field string, value int) error {
	if value > math.MaxInt32 || value < math.MinInt32 {
		return fmt.Errorf("%s is out of range", field)
	}
	return nil
}

// Uint bounds-checks an unsigned integer field (e.g. a foreign key) for the
// same overflow-guarding reason as Int.
func Uint(field string, value uint) error {
	if value > math.MaxInt32 {
		return fmt.Errorf("%s is out of range", field)
	}
	return nil
}

// IntRange bounds-checks value against [min, max] - for fields with a known,
// well-defined domain (a port number, a VLAN tag, a pf queue priority) where
// a plain overflow guard isn't tight enough.
func IntRange(field string, value, min, max int) error {
	if value < min || value > max {
		return fmt.Errorf("%s must be between %d and %d", field, min, max)
	}
	return nil
}
