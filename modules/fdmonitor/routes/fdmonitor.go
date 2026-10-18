// Package fdmonitorroutes - Arkgate API fdmonitor module
//
// Like modules/diagnostics and modules/captiveportal, there's no
// modules/fdmonitor/model: nothing is persisted, every number is read
// live off the gateway through arkgated.
//
// Two different measurements are reported, and they are not the same
// count - conflating them is the easy mistake here:
//
//   - kern.nfiles / kern.maxfiles: system-wide open file table entries
//     and the hard ceiling on them. This is the one that matters for
//     exhaustion - when nfiles hits maxfiles, the box stops being able to
//     open anything.
//
//   - per-process descriptor counts from fstat(1): how many descriptors
//     each process holds. These sum to more than kern.nfiles, because
//     several descriptors (dup'd, or inherited across fork) can share a
//     single file table entry. Useful for "what is leaking", not as a
//     second opinion on the total.
//
// Routes:
//
//	/api/v1/fdmonitor
//	  Method: GET
//	  Headers: Authorization Bearer
//	  Return: JSON {"open", "max", "available", "percent", "top":[...],
//	          "error"} - "top" lists the heaviest descriptor holders.
//	          A failure reaching arkgated is reported in "error" rather
//	          than as an HTTP error, so the page can still render what
//	          it did get.
//	  Return-Status: 200 always
package fdmonitorroutes

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	Acmd "github.com/rbaylon/arkgatecmd"
	"github.com/rbaylon/srvcman/modules/security"
	"github.com/rbaylon/srvcman/utils"
	"gorm.io/gorm"
)

const (
	sysctlBin = "/usr/sbin/sysctl"
	fstatBin  = "/usr/bin/fstat"
)

// cmdTimeout bounds how long a single sysctl/fstat run may wait on
// arkgated. fstat walks every process's descriptor table, so it's the
// slower of the two on a busy box.
const cmdTimeout = 15 * time.Second

// topProcs is how many of the heaviest descriptor holders are reported.
const topProcs = 10

// ProcFD is one process's descriptor count, as counted from fstat(1).
type ProcFD struct {
	User string `json:"user"`
	Cmd  string `json:"cmd"`
	Pid  int    `json:"pid"`
	Fds  int    `json:"fds"`
}

// Status is the whole monitor reading.
type Status struct {
	Open      int      `json:"open"`
	Max       int      `json:"max"`
	Available int      `json:"available"`
	Percent   float64  `json:"percent"`
	Top       []ProcFD `json:"top"`
	Error     string   `json:"error,omitempty"`
}

// runViaArk dispatches a read-only command to arkgated and returns its
// output.
func runViaArk(name, binpath string, opts []string) (string, error) {
	conn := utils.GetArkConn()
	if conn == nil {
		return "", fmt.Errorf("arkgated unreachable")
	}
	conn.SetDeadline(time.Now().Add(cmdTimeout))
	cmd := Acmd.PrepareCmd(name, binpath, opts)
	return cmd.SendCmdOutput(conn)
}

// parseNfiles reads `sysctl -n kern.nfiles kern.maxfiles` output: two
// integers, one per line, in that order.
func parseNfiles(out string) (open, max int, err error) {
	fields := strings.Fields(out)
	if len(fields) < 2 {
		return 0, 0, fmt.Errorf("unexpected sysctl output: %q", strings.TrimSpace(out))
	}
	open, err = strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, fmt.Errorf("kern.nfiles %q: %w", fields[0], err)
	}
	max, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, fmt.Errorf("kern.maxfiles %q: %w", fields[1], err)
	}
	return open, max, nil
}

// parseFstat counts descriptors per process from fstat(1) output,
// returning the heaviest limit holders first.
//
// fstat's FD column is either a descriptor number (sometimes suffixed
// with '*') or one of the non-descriptor entries it also prints for every
// process - "text" (the executable), "wd" (working directory), "root",
// "tr" (kernel trace). Only the numeric ones are actual descriptors, so
// counting rows blindly would inflate every process by three or so.
func parseFstat(out string, limit int) []ProcFD {
	counts := map[int]*ProcFD{}
	for i, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if i == 0 || len(fields) < 4 {
			continue
		}
		pid, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}
		if _, err := strconv.Atoi(strings.TrimSuffix(fields[3], "*")); err != nil {
			continue
		}
		p, seen := counts[pid]
		if !seen {
			p = &ProcFD{User: fields[0], Cmd: fields[1], Pid: pid}
			counts[pid] = p
		}
		p.Fds++
	}

	procs := make([]ProcFD, 0, len(counts))
	for _, p := range counts {
		procs = append(procs, *p)
	}
	sort.Slice(procs, func(i, j int) bool {
		if procs[i].Fds != procs[j].Fds {
			return procs[i].Fds > procs[j].Fds
		}
		return procs[i].Pid < procs[j].Pid
	})
	if limit > 0 && len(procs) > limit {
		procs = procs[:limit]
	}
	return procs
}

// Read gathers a full reading. A failure on either command is reported in
// Status.Error rather than aborting: the counts are still worth showing
// without the per-process breakdown, and vice versa.
func Read() Status {
	var st Status
	var errs []string

	out, err := runViaArk("FdSysctl", sysctlBin, []string{"-n", "kern.nfiles", "kern.maxfiles"})
	if err != nil {
		errs = append(errs, err.Error())
	} else if open, max, perr := parseNfiles(out); perr != nil {
		errs = append(errs, perr.Error())
	} else {
		st.Open, st.Max = open, max
		st.Available = max - open
		if max > 0 {
			st.Percent = float64(open) / float64(max) * 100
		}
	}

	if out, err := runViaArk("FdFstat", fstatBin, nil); err != nil {
		errs = append(errs, err.Error())
	} else {
		st.Top = parseFstat(out, topProcs)
	}

	st.Error = strings.Join(errs, "; ")
	return st
}

func FdMonitorRouter(db *gorm.DB) chi.Router {
	r := chi.NewRouter()
	r.Use(security.TokenRequired(db, "fdmonitor"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		render.JSON(w, r, Read())
	})

	return r
}
