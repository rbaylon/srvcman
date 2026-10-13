package changeapply

import (
	"errors"
	"strings"
	"testing"

	Acmd "github.com/rbaylon/arkgatecmd"
	Changemodel "github.com/rbaylon/srvcman/modules/changes/model"
)

// gateway records the commands Npppd sends and fails the ones it is told to.
type gateway struct {
	log      []string
	failName map[string]int // command name -> how many times it fails (0 = never)
	checkOut string
}

func (g *gateway) record(c *Acmd.Arkcmd) string {
	g.log = append(g.log, c.Name)
	return c.Name
}

func (g *gateway) send(c *Acmd.Arkcmd) error {
	name := g.record(c)
	if n := g.failName[name]; n > 0 {
		g.failName[name] = n - 1
		return errors.New("NOK")
	}
	return nil
}

func (g *gateway) output(c *Acmd.Arkcmd) (string, error) {
	name := g.record(c)
	if n := g.failName[name]; n > 0 {
		g.failName[name] = n - 1
		return g.checkOut, errors.New("exit status 1")
	}
	return "configuration OK", nil
}

func (g *gateway) calls() Calls { return Calls{Send: g.send, Output: g.output} }

func (g *gateway) sequence() string { return strings.Join(g.log, " > ") }

func TestAGoodConfigIsInstalledCheckedRestartedAndEnabled(t *testing.T) {
	g := &gateway{failName: map[string]int{}}
	errs := Npppd(g.calls())
	if len(errs) != 0 {
		t.Fatalf("errors on a clean apply: %+v", errs)
	}
	want := "BackupConf_npppd > StageConf_npppd > CheckInstalled_npppd > Enable_npppd > Restart_npppd"
	if got := g.sequence(); got != want {
		t.Errorf("sequence\n got  %s\n want %s", got, want)
	}
}

// The first install has no previous file, so setting it aside fails - that must
// not stop the install.
func TestAFailedBackupOnTheFirstInstallIsNotAnError(t *testing.T) {
	g := &gateway{failName: map[string]int{"BackupConf_npppd": 1}}
	if errs := Npppd(g.calls()); len(errs) != 0 {
		t.Errorf("first install reported errors: %+v", errs)
	}
	if !strings.Contains(g.sequence(), "Restart_npppd") {
		t.Errorf("first install never restarted npppd: %s", g.sequence())
	}
}

// This is what used to report success while npppd was dead: the config never
// arrived, the install failed, and npppd was restarted with nothing to read.
func TestIfTheConfigCannotBeInstalledNpppdIsNotTouched(t *testing.T) {
	g := &gateway{failName: map[string]int{"StageConf_npppd": 1}}
	errs := Npppd(g.calls())
	if len(errs) != 1 || errs[0].Module != "pppoes" || !strings.Contains(errs[0].Data, "could not install") {
		t.Fatalf("errors = %+v, want one pppoes error about the install", errs)
	}
	for _, c := range g.log {
		if c == "Restart_npppd" || c == "CheckInstalled_npppd" {
			t.Errorf("went on to %s after the install failed: %s", c, g.sequence())
		}
	}
	if !strings.Contains(g.sequence(), "RevertConf_npppd") {
		t.Errorf("the previous config was not put back: %s", g.sequence())
	}
}

// A config npppd refuses must never reach a restart: restarting is what stops a
// running daemon.
func TestAConfigNpppdRejectsIsNeverRestartedOn(t *testing.T) {
	g := &gateway{failName: map[string]int{"CheckInstalled_npppd": 1}, checkOut: "2026-10-13 15:23:45:CRIT: conf:11: Invalid argument\n2026-10-13 15:23:45:CRIT: conf:15: ipcp IPCP0 is not found"}
	errs := Npppd(g.calls())
	if len(errs) != 1 || !strings.Contains(errs[0].Data, "Invalid argument") || !strings.Contains(errs[0].Data, "npppd was not restarted") {
		t.Fatalf("errors = %+v, want npppd's own message and the fact it was not restarted", errs)
	}
	if strings.Contains(g.sequence(), "Restart_npppd") || strings.Contains(g.sequence(), "Enable_npppd") {
		t.Errorf("restarted or enabled on a rejected config: %s", g.sequence())
	}
	if !strings.HasSuffix(g.sequence(), "RevertConf_npppd") {
		t.Errorf("the previous config was not restored: %s", g.sequence())
	}
}

// If the restart itself fails npppd is left stopped, so after putting the old
// file back it has to be started again.
func TestAFailedRestartRestoresTheOldConfigAndStartsNpppdAgain(t *testing.T) {
	g := &gateway{failName: map[string]int{"Restart_npppd": 1}}
	errs := Npppd(g.calls())
	if len(errs) != 1 {
		t.Fatalf("errors = %+v, want exactly the restart failure (the second restart worked)", errs)
	}
	want := "BackupConf_npppd > StageConf_npppd > CheckInstalled_npppd > Enable_npppd > Restart_npppd > RevertConf_npppd > Restart_npppd"
	if got := g.sequence(); got != want {
		t.Errorf("sequence\n got  %s\n want %s", got, want)
	}
}

func TestItSaysSoWhenNpppdCannotBeBroughtBackEither(t *testing.T) {
	g := &gateway{failName: map[string]int{"Restart_npppd": 2}}
	errs := Npppd(g.calls())
	if len(errs) != 2 || !strings.Contains(errs[1].Data, "npppd is not running") {
		t.Errorf("errors = %+v, want the second to say npppd is not running", errs)
	}
}

func TestOneLineKeepsNpppdsMessage(t *testing.T) {
	if got := oneLine("a\n  b \n", errors.New("x")); got != "a; b" {
		t.Errorf("oneLine = %q", got)
	}
	if got := oneLine("   ", errors.New("exit status 1")); got != "exit status 1" {
		t.Errorf("oneLine with no output = %q", got)
	}
}

// rcctl will not restart a disabled service, so the service has to be enabled
// before the restart is tried - and a failure to enable must not stop the
// attempt, since the restart may still work.
func TestEnableComesBeforeTheRestartAndIsNotFatal(t *testing.T) {
	g := &gateway{failName: map[string]int{"Enable_npppd": 1}}
	errs := applyNpppdFor(g)
	if len(errs) != 0 {
		t.Errorf("errors = %+v, want none: a failed enable is only logged", errs)
	}
	seq := g.sequence()
	if strings.Index(seq, "Enable_npppd") > strings.Index(seq, "Restart_npppd") || !strings.Contains(seq, "Restart_npppd") {
		t.Errorf("enable did not come before the restart: %s", seq)
	}
}

func applyNpppdFor(g *gateway) []Changemodel.Change { return Npppd(g.calls()) }
