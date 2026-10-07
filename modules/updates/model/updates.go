// Package updatesmodel - software update settings and OS patch checks
package updatesmodel

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	Acmd "github.com/rbaylon/arkgatecmd"
	"github.com/rbaylon/srvcman/utils"
	"github.com/rbaylon/srvcman/utils/validate"
	"gorm.io/gorm"
)

const (
	syspatchBin = "/usr/sbin/syspatch"

	// maxServerLen bounds the stored URL; 2048 is the practical ceiling
	// for URLs in common use.
	maxServerLen = 2048

	// checkTimeout bounds how long a check waits on arkgated. `syspatch -c`
	// fetches the patch list from the network and can take minutes;
	// arkgated allows it 10 minutes (its TimeoutFor("SyspatchCheck")), so
	// this is that plus margin: arkgated's own limit, not this one, is
	// what ends a stuck run.
	checkTimeout = 11 * time.Minute

	// Check states, as reported by CheckStatus.
	StateIdle    = "idle"
	StateRunning = "running"
	StateDone    = "done"
	StateFailed  = "failed"
)

// Updates is the gateway's software update configuration. It is a
// single-entry config - one box has one update server - so there is one
// row, read and written through Get and Save rather than a list.
//
// UpdateServer is the base URL of the server the product's components are
// updated from. It is empty until an operator sets one.
type Updates struct {
	gorm.Model
	UpdateServer string `json:"update_server"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Updates{})
	if err != nil {
		log.Fatal(err)
	}
}

// ValidateServer requires value to be empty (no server configured) or an
// absolute http(s) URL. The value is stored and later used to build
// requests, so anything that is not a plain URL - whitespace, control
// characters, embedded credentials, other schemes - is refused here.
func ValidateServer(value string) error {
	if value == "" {
		return nil
	}
	if len(value) > maxServerLen {
		return fmt.Errorf("update_server exceeds maximum length of %d characters", maxServerLen)
	}
	for _, c := range value {
		if unicode.IsSpace(c) || unicode.IsControl(c) {
			return errors.New("update_server must not contain spaces or control characters")
		}
	}
	u, err := url.Parse(value)
	if err != nil {
		return errors.New("update_server is not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("update_server must start with http:// or https://")
	}
	if u.Hostname() == "" {
		return errors.New("update_server must include a host name")
	}
	if u.User != nil {
		return errors.New("update_server must not contain a user name or password")
	}
	return nil
}

// Bind interface as required by go-chi/render
func (u *Updates) Bind(r *http.Request) error {
	u.UpdateServer = strings.TrimSpace(u.UpdateServer)
	if err := validate.MaxLen("update_server", u.UpdateServer, maxServerLen); err != nil {
		return err
	}
	return ValidateServer(u.UpdateServer)
}

// Get returns the single settings row, or an empty one (ID 0) when no
// server has been saved yet.
func Get(db *gorm.DB) (*Updates, error) {
	var u Updates
	err := db.First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &Updates{}, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// Save stores server as the update server, creating the row on first use.
func Save(db *gorm.DB, server string) (*Updates, error) {
	u, err := Get(db)
	if err != nil {
		return nil, err
	}
	u.UpdateServer = server
	// Save (rather than Updates) so that clearing the field back to the
	// empty string persists - Updates skips zero values.
	if err := db.Save(u).Error; err != nil {
		return nil, err
	}
	return u, nil
}

// Patches is the outcome of an OS patch check.
type Patches struct {
	// Available lists the patches syspatch(8) reports, one per entry,
	// e.g. "001_sndio". Empty means the system is up to date.
	Available []string `json:"available"`
	// Output is syspatch's own output, unparsed.
	Output string `json:"output"`
}

// CheckState is the status of the background patch check, as reported to
// the API: whether one is running, and the outcome of the last one.
type CheckState struct {
	State      string     `json:"state"` // idle, running, done or failed
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Available  []string   `json:"available"`
	Output     string     `json:"output"`
	// Error is the failure message when State is failed: syspatch's own
	// output when it printed any, otherwise the transport error.
	Error string `json:"error,omitempty"`
}

// check is the single background patch check. It lives in memory only: a
// srvcman restart forgets the last result and reports idle, which is
// correct - the result is a snapshot of a moment, not a record to keep.
var check = struct {
	sync.Mutex
	state CheckState
}{state: CheckState{State: StateIdle, Available: []string{}}}

// runCheck is what the background job calls. A var so tests can stand in
// for arkgated.
var runCheck = CheckPatches

// CheckStatus returns a copy of the current check state.
func CheckStatus() CheckState {
	check.Lock()
	defer check.Unlock()
	return check.state
}

// StartCheck starts a patch check in the background and returns at once.
// Only one runs at a time: if one is already running it is left alone and
// started is false, so repeated clicks cannot pile up `syspatch` runs on
// the gateway.
//
// The check runs in its own goroutine, detached from any HTTP request, so
// it keeps going after the caller has been answered; progress is read back
// with CheckStatus.
func StartCheck() (state CheckState, started bool) {
	check.Lock()
	if check.state.State == StateRunning {
		state = check.state
		check.Unlock()
		return state, false
	}
	now := time.Now()
	check.state = CheckState{State: StateRunning, StartedAt: &now, Available: []string{}}
	state = check.state
	check.Unlock()

	go func() {
		var p *Patches
		var err error
		// A panic in the job must not leave the state stuck on running,
		// which would block every later check.
		defer func() {
			if r := recover(); r != nil {
				p, err = nil, fmt.Errorf("update check crashed: %v", r)
			}
			finishCheck(p, err)
		}()
		p, err = runCheck()
	}()
	return state, true
}

// finishCheck records the outcome of the running check.
func finishCheck(p *Patches, err error) {
	check.Lock()
	defer check.Unlock()
	now := time.Now()
	check.state.FinishedAt = &now
	if p != nil {
		check.state.Available = p.Available
		check.state.Output = p.Output
	}
	if err != nil {
		check.state.State = StateFailed
		check.state.Error = err.Error()
		if p != nil && p.Output != "" {
			// syspatch's own message (an unreachable mirror, a bad
			// signature) is the useful part of a failure.
			check.state.Error = p.Output
		}
		log.Println("updates: check failed:", check.state.Error)
		return
	}
	check.state.State = StateDone
}

// CheckPatches asks arkgated to run `syspatch -c` on the gateway and
// returns what it reports. syspatch has to run as root on the gateway
// itself, so srvcman never execs it directly. The returned Patches is
// populated even on error. It blocks for as long as syspatch runs - use
// StartCheck to run it in the background.
func CheckPatches() (*Patches, error) {
	conn := utils.GetArkConn()
	if conn == nil {
		return nil, fmt.Errorf("arkgated unreachable")
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(checkTimeout))
	cmd := Acmd.PrepareCmd("SyspatchCheck", syspatchBin, []string{"-c"})
	out, err := cmd.SendCmdOutput(conn)
	return parsePatches(out), err
}

// parsePatches splits syspatch -c output into patch names.
func parsePatches(out string) *Patches {
	p := &Patches{Available: []string{}, Output: strings.TrimSpace(out)}
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			p.Available = append(p.Available, line)
		}
	}
	return p
}
