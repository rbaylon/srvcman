package changeapply

import (
	"testing"

	Changemodel "github.com/rbaylon/srvcman/modules/changes/model"
)

func TestAFailedPfStepHoldsBackEveryPfChange(t *testing.T) {
	failed := Failed([]Changemodel.Change{{Module: "subs", Data: "error in /tmp/pf.conf"}})
	for _, m := range []string{"subs", "firewall", "pfifaces", "pflows"} {
		if !Held(m, failed) {
			t.Errorf("%s change was released although pf.conf was not installed", m)
		}
	}
	for _, m := range []string{"pppoes", "dhcpserver", "plans", "vouchers"} {
		if Held(m, failed) {
			t.Errorf("%s change was held by an unrelated failure", m)
		}
	}
}

func TestAFailedServiceHoldsOnlyItsOwnChanges(t *testing.T) {
	failed := Failed([]Changemodel.Change{{Module: "pppoes"}, {Module: "dhcpserver"}})
	if !Held("pppoes", failed) || !Held("dhcpserver", failed) || Held("subs", failed) || Held("snmpserver", failed) {
		t.Errorf("wrong grouping: %v", failed)
	}
}

func TestNothingIsHeldWhenNothingFailed(t *testing.T) {
	failed := Failed(nil)
	if Held("subs", failed) || Held("pppoes", failed) {
		t.Error("a clean apply held changes back")
	}
}
