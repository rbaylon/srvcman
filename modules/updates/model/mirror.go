package updatesmodel

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	Acmd "github.com/rbaylon/arkgatecmd"
	"github.com/rbaylon/srvcman/utils"
)

// The OpenBSD mirror is the one line in /etc/installurl that syspatch (and
// pkg_add, fw_update) download from. A mirror near the box makes OS updates much
// faster, so the operator can change it. The file belongs to the gateway, so it
// is read and written through arkgated, never directly.

const (
	installURLPath = "/etc/installurl"
	maxMirrorLen   = 255
	mirrorTimeout  = 45 * time.Second
)

// DefaultMirror is what OpenBSD installs with: a content network that picks a
// nearby server itself.
const DefaultMirror = "https://cdn.openbsd.org/pub/OpenBSD"

var mirrorHost = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$`)
var mirrorPath = regexp.MustCompile(`^[A-Za-z0-9._~/+-]*$`)

// ValidateMirror checks an installurl value and returns it in the form that is
// stored: no trailing slash. It must be a plain http(s) address; anything that
// could change what the shell or ftp does with it is refused, because the
// value ends up in a command that runs as root.
func ValidateMirror(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("mirror is required")
	}
	if len(value) > maxMirrorLen {
		return "", fmt.Errorf("mirror exceeds maximum length of %d characters", maxMirrorLen)
	}
	u, err := url.Parse(value)
	if err != nil {
		return "", errors.New("mirror is not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("mirror must start with http:// or https://")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(value, "?#@ \t\r\n\\'\"`$;&|<>(){}*!") {
		return "", errors.New("mirror must be a plain address without credentials, query or special characters")
	}
	if !mirrorHost.MatchString(u.Hostname()) || !strings.Contains(u.Hostname(), ".") {
		return "", errors.New("mirror must include a valid host name")
	}
	if p := u.Port(); p != "" && (len(p) > 5 || strings.Trim(p, "0123456789") != "") {
		return "", errors.New("mirror has an invalid port")
	}
	if !mirrorPath.MatchString(u.Path) || strings.Contains(u.Path, "..") {
		return "", errors.New("mirror has an invalid path")
	}
	return strings.TrimRight(value, "/"), nil
}

// mirrorRun runs one command on the gateway and returns what it printed. Tests
// replace it.
var mirrorRun = func(name, bin string, opts []string) (string, error) {
	conn := utils.GetArkConn()
	if conn == nil {
		return "", errors.New("arkgated unreachable")
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(mirrorTimeout))
	return Acmd.PrepareCmd(name, bin, opts).SendCmdOutput(conn)
}

// GetMirror returns the gateway's current mirror ("" when the file is missing).
func GetMirror() (string, error) {
	out, err := mirrorRun("MirrorGet", "/bin/cat", []string{installURLPath})
	if err != nil {
		// No file is a normal state on a fresh install.
		return "", nil
	}
	line := strings.TrimSpace(strings.SplitN(out, "\n", 2)[0])
	return line, nil
}

// setMirrorScript checks that the mirror answers (the same file syspatch
// fetches first) before it replaces the old one, so a typo or a dead mirror can
// never leave the box without working updates. The address is the script's
// first argument, never part of its text.
const setMirrorScript = `u="$0"
ftp -w 20 -o /dev/null -M "$u/$(uname -r)/$(machine)/SHA256.sig" 2>&1 || { echo "the mirror did not answer"; exit 3; }
printf '%s\n' "$u" > /etc/installurl.tmp && mv /etc/installurl.tmp /etc/installurl`

// SetMirror validates, tests and installs a mirror, and returns the stored value.
func SetMirror(value string) (string, error) {
	v, err := ValidateMirror(value)
	if err != nil {
		return "", err
	}
	out, err := mirrorRun("MirrorSet", "/bin/sh", []string{"-c", setMirrorScript, v})
	if err != nil {
		msg := strings.TrimSpace(strings.ReplaceAll(out, "\n", "; "))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("the mirror was not changed: %s", msg)
	}
	return v, nil
}
