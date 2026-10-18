package updatesmodel

import (
	"bufio"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	Acmd "github.com/rbaylon/arkgatecmd"
	"github.com/rbaylon/srvcman/utils"
)

// Software updates: checking the update server, downloading an update and
// installing it.
//
// Installing has to happen as root (OS patches, restarting services), which
// srvcman never is; it asks arkgated to run the runner script
// (specz-update.sh, embedded below) detached, and from then on only watches the
// status file the runner writes. That split is deliberate:
//
//   - the install usually restarts srvcman itself, so nothing here may depend
//     on srvcman staying up: the runner owns the work and the status file is
//     the truth, read again after any restart;
//   - arkgated limits a command's run time and size, which a download plus an
//     OS patch round exceeds, so it only starts the runner and returns.
//
// The OS is patched by the runner before the update is installed. The
// operator is not asked about it and the API only ever calls the step
// "preparing the system".
//
// This needs srvcman and arkgated on the same host (the appliance layout):
// the files srvcman downloads are read from disk by the runner.

//go:embed specz-update.sh
var runnerScriptRaw string

// runnerScript is the runner with Unix line endings whatever the checkout
// did to the file: a script with CRLF line endings does not run under
// OpenBSD's sh ("set: -: unknown option"), and a git checkout on Windows with
// autocrlf would otherwise ship exactly that. (.gitattributes also pins it to LF.)
var runnerScript = strings.ReplaceAll(runnerScriptRaw, "\r\n", "\n")

// RunnerScript returns the root runner script's text.
func RunnerScript() string { return runnerScript }

// Job states, as reported by Job.State.
const (
	JobIdle    = "idle"
	JobRunning = "running"
	JobSuccess = "success"
	JobFailed  = "failed"
)

// Job steps, as reported by Job.Step while the job is running.
const (
	StepDownloading = "downloading"
	StepPreparing   = "preparing" // checking the files and the OS
	StepInstalling  = "installing"
)

var (
	ErrNothingToInstall = errors.New("there is no update to install")
	ErrBusy             = errors.New("an update is already being installed")
)

// Job is the state of the software update job, as reported to the API.
type Job struct {
	State         string     `json:"state"`
	Step          string     `json:"step,omitempty"`
	Version       string     `json:"version,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	Message       string     `json:"message,omitempty"`
	Log           string     `json:"log,omitempty"` // tail of the runner's log, when failed
	RebootPending bool       `json:"reboot_pending"`
}

// Root starts the runner as root. arkRoot is the real one; tests replace it.
type Root interface {
	Start(script, job, stage string) error
}

type arkRoot struct{}

// Start asks arkgated to start the runner detached and return at once. The
// shell wrapper backgrounds the runner with its output redirected, which is
// what lets arkgated's command return (it waits for the output pipes to
// close) while the runner carries on, outside arkgated's time limit. The
// runner's own arguments are passed as positional parameters, never spliced
// into the command text.
func (arkRoot) Start(script, job, stage string) error {
	conn := utils.GetArkConn()
	if conn == nil {
		return errors.New("arkgated unreachable")
	}
	cmd := Acmd.PrepareCmd("SpeczUpdateStart", "/bin/sh",
		[]string{"-c", `nohup /bin/sh "$0" "$@" >/dev/null 2>&1 </dev/null &`, script, job, stage})
	return cmd.SendCmd(conn)
}

// Manager runs the software update job. There is one per process (Default);
// tests make their own.
type Manager struct {
	Root Root
	// Dir is the updates directory: staging/ (downloads), jobs/ (runner
	// status and logs) and the runner script. "" means the default.
	Dir string
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// Client makes the update server client; nil means NewClient. Tests use it
	// to point at a fake server.
	Client func(server, product, token string) *Client

	mu  sync.Mutex
	mem struct {
		job   string
		phase string // "downloading", "started" or "failed"
		err   string
		since time.Time
	}
}

// Default is the manager srvcman's routes use.
var Default = &Manager{Root: arkRoot{}}

const defaultDir = "/usr/local/arkgate/updates"

func (m *Manager) dir() string {
	if m.Dir != "" {
		return m.Dir
	}
	if d := os.Getenv("UPDATES_DIR"); d != "" {
		return d
	}
	return defaultDir
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) client(server, product, token string) *Client {
	if m.Client != nil {
		return m.Client(server, product, token)
	}
	return NewClient(server, product, token)
}

// installedRelease is the release the box was installed from, from the file the
// installer image writes (RELEASE_FILE, default /usr/local/arkgate/RELEASE).
// "" when unknown: the update server is then asked for its latest release.
func installedRelease() string {
	path := os.Getenv("RELEASE_FILE")
	if path == "" {
		path = "/usr/local/arkgate/RELEASE"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if v := strings.TrimSpace(string(b)); versionRE.MatchString(v) {
		return v
	}
	return ""
}

// Software is what a software update check reports.
type Software struct {
	// Configured is false until an update server and token are saved; the
	// other fields are then empty and nothing is wrong.
	Configured bool   `json:"configured"`
	Product    string `json:"product"`
	// Release is the release the box runs ("" if unknown) and Installed the
	// version of the last update installed ("" if none).
	Release   string `json:"release"`
	Installed string `json:"installed"`
	// Tier is the plan of the token's account at the update server: free or pro.
	Tier            string     `json:"tier,omitempty"`
	Latest          *Update    `json:"latest"`
	Scheduled       *Scheduled `json:"scheduled,omitempty"`
	UpdateAvailable bool       `json:"update_available"`
	// Error is why the check failed, written for the operator.
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// CheckSoftware asks the update server for the latest update. A failure of
// the check itself is reported in Software.Error, not as an error return, so
// the page can show it next to what it does know.
func (m *Manager) CheckSoftware(ctx context.Context, st Store) (*Software, error) {
	u, err := st.Load()
	if err != nil {
		return nil, err
	}
	s := &Software{Product: productOf(u), Release: installedRelease(), Installed: u.InstalledVersion, CheckedAt: m.now()}
	if u.UpdateServer == "" || u.ApiToken == "" {
		return s, nil
	}
	s.Configured = true
	l, err := m.client(u.UpdateServer, s.Product, u.ApiToken).Latest(ctx, s.Release)
	if err != nil {
		s.Error = capitalise(err.Error())
		return s, nil
	}
	s.Tier, s.Latest, s.Scheduled = l.Tier, l.Latest, l.Scheduled
	if s.Release == "" {
		s.Release = l.Release
	}
	s.UpdateAvailable = l.Latest != nil && l.Latest.Version != u.InstalledVersion
	return s, nil
}

func productOf(u *Updates) string {
	if u.Product == "" {
		return DefaultProduct
	}
	return u.Product
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// StartInstall starts installing the latest update in the background and returns
// the job's state at once. If one is already running it is left alone.
func (m *Manager) StartInstall(ctx context.Context, st Store) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j := m.jobLocked(st); j.State == JobRunning {
		return j, nil
	}
	u, err := st.Load()
	if err != nil {
		return nil, err
	}
	if u.UpdateServer == "" || u.ApiToken == "" {
		return nil, ErrNotConfigured
	}
	product := productOf(u)
	cl := m.client(u.UpdateServer, product, u.ApiToken)
	l, err := cl.Latest(ctx, installedRelease())
	if err != nil {
		return nil, err
	}
	if l.Latest == nil || l.Latest.Version == u.InstalledVersion {
		return nil, ErrNothingToInstall
	}
	update := l.Latest

	id, err := newJobID(m.now())
	if err != nil {
		return nil, err
	}
	stage := filepath.Join(m.dir(), "staging", id)
	jobDir := filepath.Join(m.dir(), "jobs", id)
	for _, d := range []string{stage, jobDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("could not prepare the update directory: %w", err)
		}
	}
	// The staging directory holds the downloads: only srvcman (and root, which
	// reads them) needs to see inside.
	if err := os.Chmod(stage, 0o700); err != nil {
		return nil, err
	}
	script := filepath.Join(m.dir(), "specz-update.sh")
	if err := writeFileAtomic(script, []byte(runnerScript), 0o755); err != nil {
		return nil, fmt.Errorf("could not write the update runner: %w", err)
	}
	started := m.now()
	u.JobID, u.JobVersion, u.JobStartedAt, u.JobRecorded = id, update.Version, &started, false
	if err := st.Save(u); err != nil {
		return nil, err
	}
	m.mem.job, m.mem.phase, m.mem.err, m.mem.since = id, "downloading", "", started
	m.prune(id)

	go m.run(cl, update, id, stage, script)
	return m.jobLocked(st), nil
}

// run downloads the update, then hands over to the runner. It is detached
// from any request: the operator may close the page.
func (m *Manager) run(cl *Client, up *Update, id, stage, script string) {
	fail := func(err error) {
		log.Println("updates: software update failed before install:", err)
		os.RemoveAll(stage)
		m.mu.Lock()
		if m.mem.job == id {
			m.mem.phase, m.mem.err = "failed", capitalise(err.Error())
		}
		m.mu.Unlock()
	}
	defer func() {
		if r := recover(); r != nil {
			fail(fmt.Errorf("the update crashed: %v", r))
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	var manifest strings.Builder
	for _, f := range up.Files {
		if err := cl.Download(ctx, f, stage); err != nil {
			fail(err)
			return
		}
		fmt.Fprintf(&manifest, "%s  %s\n", f.SHA256, f.Name)
	}
	if err := os.WriteFile(filepath.Join(stage, "MANIFEST"), []byte(manifest.String()), 0o600); err != nil {
		fail(err)
		return
	}
	if err := os.WriteFile(filepath.Join(stage, "VERSION"), []byte(up.Version+"\n"), 0o600); err != nil {
		fail(err)
		return
	}
	if err := m.Root.Start(script, id, stage); err != nil {
		fail(fmt.Errorf("could not start the installation: %w", err))
		return
	}
	m.mu.Lock()
	if m.mem.job == id {
		m.mem.phase, m.mem.since = "started", m.now()
	}
	m.mu.Unlock()
}

// JobStatus reports the software update job.
func (m *Manager) JobStatus(st Store) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jobLocked(st)
}

// startGrace is how long after the runner was started its status file may still
// be missing before the job counts as having failed to start.
const startGrace = 2 * time.Minute

// staleAfter is how long a running job's status may go without a sign of life
// from the runner before the job counts as interrupted (a reboot, a kill): the
// runner process is checked too, this only covers a pid that was reused.
const staleAfter = 30 * time.Minute

func (m *Manager) jobLocked(st Store) *Job {
	u, err := st.Load()
	if err != nil || u.JobID == "" {
		return &Job{State: JobIdle}
	}
	j := &Job{State: JobRunning, Version: u.JobVersion, StartedAt: u.JobStartedAt}
	if m.mem.job == u.JobID {
		switch m.mem.phase {
		case "downloading":
			j.Step = StepDownloading
			return j
		case "failed":
			fin := m.mem.since
			j.State, j.Message, j.FinishedAt = JobFailed, m.mem.err, &fin
			return j
		}
	}
	// Handed to the runner (or srvcman restarted since): the status file is the truth.
	status, ok := readStatus(filepath.Join(m.dir(), "jobs", u.JobID, "status"))
	if !ok {
		// No status yet: the runner was only just started, or never was
		// (srvcman restarted while downloading, arkgated refused).
		since := u.JobStartedAt
		if m.mem.job == u.JobID && m.mem.phase == "started" {
			s := m.mem.since
			since = &s
		}
		if since != nil && m.now().Sub(*since) < startGrace {
			j.Step = StepPreparing
			return j
		}
		j.State, j.Message = JobFailed, "The installation did not start."
		return j
	}
	j.RebootPending = status["reboot_pending"] == "1"
	if ts, err := strconv.ParseInt(status["updated"], 10, 64); err == nil {
		t := time.Unix(ts, 0)
		j.FinishedAt = &t
	}
	switch status["state"] {
	case "success":
		j.State, j.Message = JobSuccess, status["message"]
		m.record(st, u)
	case "failed":
		j.State, j.Message = JobFailed, status["message"]
		j.Log = tailFile(filepath.Join(m.dir(), "jobs", u.JobID, "log"), 40, 4096)
	default:
		j.FinishedAt = nil
		switch status["step"] {
		case "install":
			j.Step = StepInstalling
		default:
			j.Step = StepPreparing
		}
		pid, _ := strconv.Atoi(status["pid"])
		upd, _ := strconv.ParseInt(status["updated"], 10, 64)
		if (pid > 0 && !pidAlive(pid)) || m.now().Sub(time.Unix(upd, 0)) > staleAfter {
			j.State, j.Message = JobFailed, "The update was interrupted."
			j.Log = tailFile(filepath.Join(m.dir(), "jobs", u.JobID, "log"), 40, 4096)
		}
	}
	return j
}

// record folds a successful job into the installed version, once.
func (m *Manager) record(st Store, u *Updates) {
	if u.JobRecorded {
		return
	}
	now := m.now()
	u.InstalledVersion, u.InstalledAt, u.JobRecorded = u.JobVersion, &now, true
	if err := st.Save(u); err != nil {
		log.Println("updates: recording the installed version failed:", err)
	}
}

// readStatus reads the runner's key=value status file.
func readStatus(path string) (map[string]string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	st := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "="); ok {
			st[k] = v
		}
	}
	return st, st["state"] != ""
}

// tailFile returns the last lines of a file, at most maxBytes of them.
func tailFile(path string, lines, maxBytes int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	all := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	out := strings.Join(all, "\n")
	if len(out) > maxBytes {
		out = out[len(out)-maxBytes:]
	}
	return out
}

func newJobID(now time.Time) (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return now.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b), nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// prune removes all but the newest few jobs' directories (and any staging left by
// older jobs), keeping the one named keep. Job ids start with their start time,
// so name order is age order.
func (m *Manager) prune(keep string) {
	const keepJobs = 5
	for _, sub := range []string{"jobs", "staging"} {
		entries, err := os.ReadDir(filepath.Join(m.dir(), sub))
		if err != nil {
			continue
		}
		var names []string
		for _, e := range entries {
			if e.IsDir() {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for len(names) > keepJobs {
			if names[0] != keep {
				os.RemoveAll(filepath.Join(m.dir(), sub, names[0]))
			}
			names = names[1:]
		}
	}
}
