package changeapply

import Changemodel "github.com/rbaylon/srvcman/modules/changes/model"

// groupOf names the apply step each kind of change goes through. Subscribers,
// firewall rules, interfaces and flows are all rendered into the one pf.conf,
// so a pf failure holds back all of them; every other module is its own step.
var groupOf = map[string]string{
	"subs":     "pf",
	"firewall": "pf",
	"pfifaces": "pf",
	"pflows":   "pf",
}

func group(module string) string {
	if g, ok := groupOf[module]; ok {
		return g
	}
	return module
}

// Failed returns the apply steps that reported a problem.
func Failed(errs []Changemodel.Change) map[string]bool {
	failed := make(map[string]bool)
	for _, e := range errs {
		failed[group(e.Module)] = true
	}
	return failed
}

// Held reports whether a pending change of this module must stay pending
// because its apply step failed: marking it applied would hide it from the
// operator while the gateway still runs the old configuration.
func Held(module string, failed map[string]bool) bool {
	return failed[group(module)]
}
