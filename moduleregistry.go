package main

import (
	"encoding/json"
	"log"
	"os"
	"sort"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	acmemodel "github.com/rbaylon/srvcman/modules/acme/model"
	acmeroutes "github.com/rbaylon/srvcman/modules/acme/routes"
	billportalmodel "github.com/rbaylon/srvcman/modules/billportal/model"
	billportalroutes "github.com/rbaylon/srvcman/modules/billportal/routes"
	captiveportalmodel "github.com/rbaylon/srvcman/modules/captiveportal/model"
	captiveportalroutes "github.com/rbaylon/srvcman/modules/captiveportal/routes"
	changemodel "github.com/rbaylon/srvcman/modules/changes/model"
	changeroutes "github.com/rbaylon/srvcman/modules/changes/routes"
	collectionmodel "github.com/rbaylon/srvcman/modules/collections/model"
	collectionroutes "github.com/rbaylon/srvcman/modules/collections/routes"
	dhcpmodel "github.com/rbaylon/srvcman/modules/dhcps/model"
	dhcproutes "github.com/rbaylon/srvcman/modules/dhcps/routes"
	dhcpservermodel "github.com/rbaylon/srvcman/modules/dhcpserver/model"
	dhcpserverroutes "github.com/rbaylon/srvcman/modules/dhcpserver/routes"
	diagnosticsroutes "github.com/rbaylon/srvcman/modules/diagnostics/routes"
	fdmonitorroutes "github.com/rbaylon/srvcman/modules/fdmonitor/routes"
	fwmodel "github.com/rbaylon/srvcman/modules/firewall/model"
	fwroutes "github.com/rbaylon/srvcman/modules/firewall/routes"
	gatewaymonitormodel "github.com/rbaylon/srvcman/modules/gatewaymonitor/model"
	gatewaymonitorroutes "github.com/rbaylon/srvcman/modules/gatewaymonitor/routes"
	groupmodel "github.com/rbaylon/srvcman/modules/groups/model"
	grouproutes "github.com/rbaylon/srvcman/modules/groups/routes"
	netblocksmodel "github.com/rbaylon/srvcman/modules/netblocks/model"
	netblocksroutes "github.com/rbaylon/srvcman/modules/netblocks/routes"
	paymentmodel "github.com/rbaylon/srvcman/modules/payments/model"
	paymentroutes "github.com/rbaylon/srvcman/modules/payments/routes"
	pfconfigmodel "github.com/rbaylon/srvcman/modules/pfconfig/model"
	pfconfigroutes "github.com/rbaylon/srvcman/modules/pfconfig/routes"
	pfifacemodel "github.com/rbaylon/srvcman/modules/pfifaces/model"
	pfifaceroutes "github.com/rbaylon/srvcman/modules/pfifaces/routes"
	pflowmodel "github.com/rbaylon/srvcman/modules/pflows/model"
	pflowroutes "github.com/rbaylon/srvcman/modules/pflows/routes"
	planmodel "github.com/rbaylon/srvcman/modules/plans/model"
	planroutes "github.com/rbaylon/srvcman/modules/plans/routes"
	policymodel "github.com/rbaylon/srvcman/modules/policies/model"
	policyroutes "github.com/rbaylon/srvcman/modules/policies/routes"
	pppoemodel "github.com/rbaylon/srvcman/modules/pppoes/model"
	pppoeroutes "github.com/rbaylon/srvcman/modules/pppoes/routes"
	runtimemodel "github.com/rbaylon/srvcman/modules/runtimes/model"
	runtimeroutes "github.com/rbaylon/srvcman/modules/runtimes/routes"
	staticroutesmodel "github.com/rbaylon/srvcman/modules/staticroutes/model"
	staticroutesroutes "github.com/rbaylon/srvcman/modules/staticroutes/routes"
	snmpservermodel "github.com/rbaylon/srvcman/modules/snmpserver/model"
	snmpserverroutes "github.com/rbaylon/srvcman/modules/snmpserver/routes"
	submodel "github.com/rbaylon/srvcman/modules/subs/model"
	subroutes "github.com/rbaylon/srvcman/modules/subs/routes"
	tunnelmodel "github.com/rbaylon/srvcman/modules/tunnels/model"
	tunnelroutes "github.com/rbaylon/srvcman/modules/tunnels/routes"
	unbounddnsmodel "github.com/rbaylon/srvcman/modules/unbounddns/model"
	unbounddnsroutes "github.com/rbaylon/srvcman/modules/unbounddns/routes"
	updatesmodel "github.com/rbaylon/srvcman/modules/updates/model"
	updatesroutes "github.com/rbaylon/srvcman/modules/updates/routes"
	usermodel "github.com/rbaylon/srvcman/modules/users/model"
	userroutes "github.com/rbaylon/srvcman/modules/users/routes"
	vlanmodel "github.com/rbaylon/srvcman/modules/vlans/model"
	vlanroutes "github.com/rbaylon/srvcman/modules/vlans/routes"
	vouchermodel "github.com/rbaylon/srvcman/modules/vouchers/model"
	voucherroutes "github.com/rbaylon/srvcman/modules/vouchers/routes"
)

// Module ties a name (used in the module config file) to the work needed to
// bring it up: migrate its table(s) and mount its router.
type Module struct {
	Name string
	Load func(r chi.Router, db *gorm.DB)
}

// coreModules are always migrated and mounted, regardless of the module
// config file. "changes" is included here (rather than left optional) because
// modules/security's audittrail() writes to it on every authorized non-GET
// request - without its table, the audit trail breaks even for deployments
// that don't enable any optional module.
var coreModules = []Module{
	{"users", func(r chi.Router, db *gorm.DB) {
		usermodel.MigrateDB(db)
		r.Mount("/api/v1/users", userroutes.UserRouter(usermodel.New(db)))
	}},
	{"groups", func(r chi.Router, db *gorm.DB) {
		groupmodel.MigrateDB(db)
		r.Mount("/api/v1/groups", grouproutes.GroupRouter(groupmodel.New(db)))
	}},
	{"policies", func(r chi.Router, db *gorm.DB) {
		policymodel.MigrateDB(db)
		policymodel.SeedDefaultViewPolicies(db)
		r.Mount("/api/v1/policies", policyroutes.PolicyRouter(policymodel.New(db)))
	}},
	{"pfconfig", func(r chi.Router, db *gorm.DB) {
		pfconfigmodel.MigrateDB(db)
		r.Mount("/api/v1/pfconfig", pfconfigroutes.PfconfigRouter(pfconfigmodel.New(db)))
	}},
	{"pfifaces", func(r chi.Router, db *gorm.DB) {
		pfifacemodel.MigrateDB(db)
		r.Mount("/api/v1/pfiface", pfifaceroutes.PfifaceRouter(pfifacemodel.New(db)))
	}},
	{"subs", func(r chi.Router, db *gorm.DB) {
		submodel.MigrateDB(db)
		r.Mount("/api/v1/subs", subroutes.SubRouter(submodel.New(db)))
	}},
	{"plans", func(r chi.Router, db *gorm.DB) {
		planmodel.MigrateDB(db)
		r.Mount("/api/v1/plans", planroutes.PlanRouter(planmodel.New(db)))
	}},
	{"runtimes", func(r chi.Router, db *gorm.DB) {
		runtimemodel.MigrateDB(db)
		r.Mount("/api/v1/runtime", runtimeroutes.RuntimeRouter(runtimemodel.New(db)))
	}},
	{"vlans", func(r chi.Router, db *gorm.DB) {
		vlanmodel.MigrateDB(db)
		r.Mount("/api/v1/vlans", vlanroutes.VlanRouter(vlanmodel.New(db)))
	}},
	{"changes", func(r chi.Router, db *gorm.DB) {
		changemodel.MigrateDB(db)
		r.Mount("/api/v1/changes", changeroutes.ChangeRouter(changemodel.New(db)))
	}},
	// diagnostics has no model - nothing to migrate, it just triggers
	// ping/traceroute and returns their output.
	{"diagnostics", func(r chi.Router, db *gorm.DB) {
		r.Mount("/api/v1/diagnostics", diagnosticsroutes.DiagnosticsRouter(db))
	}},
	{"acme", func(r chi.Router, db *gorm.DB) {
		acmemodel.MigrateDB(db)
		r.Mount("/api/v1/acme", acmeroutes.AcmeRouter(acmemodel.New(db)))
	}},
	{"updates", func(r chi.Router, db *gorm.DB) {
		updatesmodel.MigrateDB(db)
		r.Mount("/api/v1/updates", updatesroutes.UpdatesRouter(db))
	}},
}

// optionalModules may be enabled by listing their name in the "modules" array
// of the module config file (see LoadModuleNames). Keys match the module's
// mounted URL path segment under /api/v1/, not necessarily its modules/
// directory name (e.g. "pppoe" for modules/pppoes, "pflow" for modules/pflows).
var optionalModules = map[string]Module{
	"vouchers": {"vouchers", func(r chi.Router, db *gorm.DB) {
		vouchermodel.MigrateDB(db)
		r.Mount("/api/v1/vouchers", voucherroutes.VoucherRouter(vouchermodel.New(db)))
	}},
	"dhcps": {"dhcps", func(r chi.Router, db *gorm.DB) {
		dhcpmodel.MigrateDB(db)
		r.Mount("/api/v1/dhcps", dhcproutes.DhcpRouter(dhcpmodel.New(db)))
	}},
	"dhcpserver": {"dhcpserver", func(r chi.Router, db *gorm.DB) {
		dhcpservermodel.MigrateDB(db)
		r.Mount("/api/v1/dhcpserver", dhcpserverroutes.DhcpServerRouter(dhcpservermodel.New(db)))
	}},
	"unbounddns": {"unbounddns", func(r chi.Router, db *gorm.DB) {
		unbounddnsmodel.MigrateDB(db)
		r.Mount("/api/v1/unbounddns", unbounddnsroutes.UnboundDnsRouter(unbounddnsmodel.New(db)))
	}},
	"snmpserver": {"snmpserver", func(r chi.Router, db *gorm.DB) {
		snmpservermodel.MigrateDB(db)
		r.Mount("/api/v1/snmpserver", snmpserverroutes.SnmpServerRouter(snmpservermodel.New(db)))
	}},
	"firewall": {"firewall", func(r chi.Router, db *gorm.DB) {
		fwmodel.MigrateDB(db)
		r.Mount("/api/v1/firewall", fwroutes.FwRouter(fwmodel.New(db)))
	}},
	"payments": {"payments", func(r chi.Router, db *gorm.DB) {
		paymentmodel.MigrateDB(db)
		r.Mount("/api/v1/payments", paymentroutes.PaymentRouter(paymentmodel.New(db)))
	}},
	"tunnels": {"tunnels", func(r chi.Router, db *gorm.DB) {
		tunnelmodel.MigrateDB(db)
		r.Mount("/api/v1/tunnels", tunnelroutes.TunnelRouter(tunnelmodel.New(db)))
	}},
	// netblocks keeps speedtest network blocks both in the database and in
	// the gateway's fast.com.blocks file (edited through arkgated).
	"netblocks": {"netblocks", func(r chi.Router, db *gorm.DB) {
		netblocksmodel.MigrateDB(db)
		r.Mount("/api/v1/netblocks", netblocksroutes.NetblocksRouter(db))
	}},
	"collections": {"collections", func(r chi.Router, db *gorm.DB) {
		collectionmodel.MigrateDB(db)
		r.Mount("/api/v1/collections", collectionroutes.CollectionRouter(collectionmodel.New(db)))
	}},
	"gatewaymonitor": {"gatewaymonitor", func(r chi.Router, db *gorm.DB) {
		gatewaymonitormodel.MigrateDB(db)
		gatewaymonitormodel.ApplyAllRoutes(db)
		gatewaymonitormodel.StartAllMonitors(db)
		r.Mount("/api/v1/gatewaymonitor", gatewaymonitorroutes.GatewayMonitorRouter(gatewaymonitormodel.New(db)))
	}},
	"staticroutes": {"staticroutes", func(r chi.Router, db *gorm.DB) {
		staticroutesmodel.MigrateDB(db)
		r.Mount("/api/v1/staticroutes", staticroutesroutes.StaticRouteRouter(staticroutesmodel.New(db)))
	}},
	"pppoe": {"pppoe", func(r chi.Router, db *gorm.DB) {
		pppoemodel.MigrateDB(db)
		r.Mount("/api/v1/pppoe", pppoeroutes.PppoeRouter(pppoemodel.New(db)))
	}},
	"pflow": {"pflow", func(r chi.Router, db *gorm.DB) {
		pflowmodel.MigrateDB(db)
		r.Mount("/api/v1/pflow", pflowroutes.PflowRouter(pflowmodel.New(db)))
	}},
	// captiveportal both configures the separate subsportal app (rendering
	// its .env, which arkgated installs) and controls its "captiveportal"
	// rc.d service via rcctl.
	"captiveportal": {"captiveportal", func(r chi.Router, db *gorm.DB) {
		captiveportalmodel.MigrateDB(db)
		r.Mount("/api/v1/captiveportal", captiveportalroutes.CaptivePortalRouter(captiveportalmodel.New(db)))
	}},
	// billportal is the same shape as captiveportal: it both configures
	// the separate billportal app (rendering its .env, which arkgated
	// installs) and controls its "billportal" rc.d service via rcctl.
	"billportal": {"billportal", func(r chi.Router, db *gorm.DB) {
		billportalmodel.MigrateDB(db)
		r.Mount("/api/v1/billportal", billportalroutes.BillPortalRouter(billportalmodel.New(db)))
	}},
	// fdmonitor has no model either - it only reads kern.nfiles/maxfiles
	// and fstat off the gateway through arkgated.
	"fdmonitor": {"fdmonitor", func(r chi.Router, db *gorm.DB) {
		r.Mount("/api/v1/fdmonitor", fdmonitorroutes.FdMonitorRouter(db))
	}},
}

// ModuleConfig is the shape of the JSON module config file (default: cmd.json).
type ModuleConfig struct {
	Modules []string `json:"modules"`
}

// ActiveModuleNames returns every module name actually mounted in this run:
// every core module (always on) plus whichever of enabled's names are real
// entries in optionalModules. An unknown name in the config file is
// already skipped when mounting (see main.go's loop over enabledModules)
// rather than trusted as-is, so it's skipped here identically - this list
// is meant to reflect what this deployment really has running, not every
// name someone could put in the config file.
//
// This backs the /api/v1/modules route, which exists so srvcmanui's policy
// form can offer exactly the module names a Policy.Type can meaningfully
// reference, without srvcmanui having to hardcode (and fall out of sync
// with) srvcman's own module list.
func ActiveModuleNames(enabled []string) []string {
	names := make([]string, 0, len(coreModules)+len(enabled))
	for _, m := range coreModules {
		names = append(names, m.Name)
	}
	for _, name := range enabled {
		if m, ok := optionalModules[name]; ok {
			names = append(names, m.Name)
		}
	}
	sort.Strings(names)
	return names
}

// LoadModuleNames reads the module config file at path and returns the list
// of optional module names to enable. A missing or empty file is treated as
// "no optional modules enabled" rather than a fatal error.
func LoadModuleNames(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Module config %s not found, no optional modules loaded", path)
			return nil
		}
		log.Fatalf("Failed to read module config %s: %v", path, err)
	}
	if len(data) == 0 {
		return nil
	}
	var cfg ModuleConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("Failed to parse module config %s: %v", path, err)
	}
	return cfg.Modules
}
