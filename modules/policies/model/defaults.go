package policymodel

import (
	"errors"
	"log"

	"gorm.io/gorm"
)

// defaultViewPolicies are the baseline read-only ("RO", Resource "any")
// policies covering every backend module srvcmanui's dashboard calls:
// pfifaces (interface list, traffic stats), subs and plans (dashboard
// stat charts), gatewaymonitor (gateway status card), and Changes
// (pending-changes list). None of these grant access on their own - a
// Policy only takes effect once an admin attaches a Group to it (via the
// policies UI) whose members should get that view.
//
// Type is spelled to match each module's own TokenRequired(dbconn, ...)
// call exactly, including "Changes" (capital C) for modules/changes -
// getting that wrong silently breaks the policy match in
// modules/security's validateToken, since it's a plain string comparison.
var defaultViewPolicies = []Policy{
	{Name: "DefaultViewPfifaces", Type: "pfifaces", Resource: "any", Action: "RO"},
	{Name: "DefaultViewSubs", Type: "subs", Resource: "any", Action: "RO"},
	{Name: "DefaultViewPlans", Type: "plans", Resource: "any", Action: "RO"},
	{Name: "DefaultViewGatewaymonitor", Type: "gatewaymonitor", Resource: "any", Action: "RO"},
	{Name: "DefaultViewChanges", Type: "Changes", Resource: "any", Action: "RO"},
	{Name: "DefaultViewPfconfig", Type: "pfconfig", Resource: "any", Action: "RO"},
}

// SeedDefaultViewPolicies creates whichever of defaultViewPolicies don't
// already exist (matched by Name), leaving existing ones untouched - so a
// policy an admin has since edited (e.g. to attach a group, or to narrow
// Resource) is never overwritten. Call this once at startup, after
// MigrateDB.
func SeedDefaultViewPolicies(db *gorm.DB) {
	for _, p := range defaultViewPolicies {
		var existing Policy
		err := db.Where("Name = ?", p.Name).First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Println("policies: failed to check default view policy", p.Name, ":", err)
			continue
		}
		toCreate := p
		if err := db.Create(&toCreate).Error; err != nil {
			log.Println("policies: failed to create default view policy", p.Name, ":", err)
		}
	}
}

// GetDefaultPolicies returns the current DB records for the default view
// policies (matched by Name, see defaultViewPolicies), with their Groups
// preloaded. This reflects live state - IDs and whatever Groups an admin
// has since attached - rather than the static seed definitions, so a
// record missing entirely (e.g. deleted after seeding) is simply absent
// from the result instead of being reported as if it still existed.
func GetDefaultPolicies(db *gorm.DB) ([]Policy, error) {
	names := make([]string, len(defaultViewPolicies))
	for i, p := range defaultViewPolicies {
		names[i] = p.Name
	}
	var policies []Policy
	result := db.Preload("Groups").Where("Name IN ?", names).Find(&policies)
	if result.Error != nil {
		return nil, result.Error
	}
	return policies, nil
}
