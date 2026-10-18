//go:build windows

package updatesmodel

// pidAlive: srvcman runs on OpenBSD; on Windows (development builds) there is
// no way to ask, so the runner is assumed to be alive and the staleness limit
// in jobLocked is what ends a dead job.
func pidAlive(pid int) bool { return true }
