package rcsvc

import (
	"errors"
	"testing"

	Acmd "github.com/rbaylon/arkgatecmd"
)

func TestEnabledMatchesWholeNames(t *testing.T) {
	list := "arkgated\nnpppd\nunbound\nsndiod\n"
	if !Enabled(list, "npppd") || !Enabled(list, "unbound") {
		t.Error("an enabled service was not found")
	}
	if Enabled(list, "dhcpd") || Enabled(list, "unbou") || Enabled(list, "") {
		t.Error("a service that is not enabled (or a partial name) matched")
	}
}

type fake struct {
	log      []string
	listOn   string
	listErr  error
	enableOK bool
}

func (f *fake) send(c *Acmd.Arkcmd) error {
	f.log = append(f.log, c.Name)
	if !f.enableOK {
		return errors.New("NOK")
	}
	return nil
}

func (f *fake) output(c *Acmd.Arkcmd) (string, error) {
	f.log = append(f.log, c.Name)
	return f.listOn, f.listErr
}

func TestAServiceThatIsNotEnabledIsEnabled(t *testing.T) {
	f := &fake{listOn: "arkgated\nunbound\n", enableOK: true}
	changed, err := ensure("dhcpd", f.send, f.output)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if got := f.log; len(got) != 2 || got[0] != "RcListOn_dhcpd" || got[1] != "Enable_dhcpd" {
		t.Errorf("calls: %v", got)
	}
}

func TestAnEnabledServiceIsLeftAlone(t *testing.T) {
	f := &fake{listOn: "dhcpd\nunbound\n", enableOK: true}
	changed, err := ensure("dhcpd", f.send, f.output)
	if err != nil || changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if len(f.log) != 1 {
		t.Errorf("enable was sent for an enabled service: %v", f.log)
	}
}

func TestAnUnreadableListStillTriesToEnable(t *testing.T) {
	f := &fake{listErr: errors.New("exit 1"), enableOK: true}
	changed, err := ensure("snmpd", f.send, f.output)
	if err != nil || !changed || f.log[len(f.log)-1] != "Enable_snmpd" {
		t.Errorf("changed=%v err=%v calls=%v", changed, err, f.log)
	}
}

func TestAFailedEnableIsReported(t *testing.T) {
	f := &fake{listOn: "", enableOK: false}
	if _, err := ensure("unbound", f.send, f.output); err == nil {
		t.Error("a failed enable was not reported")
	}
}
