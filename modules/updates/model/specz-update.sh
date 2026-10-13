#!/bin/sh
# Specz software update runner.
#
# Started by srvcman, detached and as root (through arkgated), once the
# update files are downloaded and checked. srvcman writes this file to
# <updates dir>/specz-update.sh from its own copy, so the runner always matches
# the srvcman that starts it.
#
#   specz-update.sh JOB STAGEDIR
#
# STAGEDIR holds the downloaded files, a MANIFEST ("<sha256>  <name>" lines)
# and a VERSION file. Progress goes to <updates dir>/jobs/JOB/status (key=value
# lines) and details to .../log; srvcman only reads them.
#
# Steps: check the files again, bring the operating system up to date, then
# unpack the update and run its install.sh. The OS step comes first so the new
# software always lands on a patched system.
#
# POSIX sh only (OpenBSD's /bin/sh is ksh).

set -u
umask 022

BASE=${SPECZ_UPDATES_DIR:-${0%/*}}
JOB=${1:-}
STAGE=${2:-}

case "$JOB" in
'' | *[!A-Za-z0-9._-]*) echo "specz-update: bad job id" >&2; exit 64 ;;
esac
case "$STAGE" in
"$BASE"/staging/*) ;;
*) echo "specz-update: bad staging directory" >&2; exit 64 ;;
esac
case "$STAGE" in
*..*) echo "specz-update: bad staging directory" >&2; exit 64 ;;
esac

JOBDIR="$BASE/jobs/$JOB"
LOG="$JOBDIR/log"
[ -d "$JOBDIR" ] || { echo "specz-update: no job directory" >&2; exit 65; }
VERSION=$(cat "$STAGE/VERSION" 2>/dev/null)

STEP=prepare
REBOOT=0
DONE=0

# put STATE STEP MESSAGE: replace the status file in one rename, so a reader
# never sees half of it.
put() {
	msg=$(printf '%s' "$3" | tr '\r\n' '  ')
	printf 'state=%s\nstep=%s\nmessage=%s\nreboot_pending=%s\npid=%s\nupdated=%s\n' \
		"$1" "$2" "$msg" "$REBOOT" "$$" "$(date +%s)" >"$JOBDIR/status.tmp" &&
		mv -f "$JOBDIR/status.tmp" "$JOBDIR/status"
}

log() { printf '%s %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >>"$LOG"; }

fail() {
	log "FAILED ($STEP): $1"
	put failed "$STEP" "$1"
	DONE=1
	exit 1
}

# Whatever kills the runner, the status must not stay "running" for ever.
trap '[ "$DONE" = 1 ] || put failed "$STEP" "The update was interrupted."' EXIT
trap 'log "terminated"; exit 1' HUP INT TERM

: >"$LOG"
log "update $VERSION started (job $JOB)"

# 1. The files are what was downloaded and checked. srvcman checked them as
# they came in; checking again here costs nothing and keeps this script safe to
# run on its own.
STEP=prepare
put running prepare "Preparing the update"
[ -f "$STAGE/MANIFEST" ] || fail "The update files are missing."
while read -r sum name; do
	[ -n "$name" ] || continue
	case "$name" in
	*/* | .* | MANIFEST | VERSION) fail "An update file has an unusable name." ;;
	esac
	[ -f "$STAGE/$name" ] || fail "The update file $name is missing."
	got=$(sha256 -q "$STAGE/$name" 2>>"$LOG") || fail "Could not check the update file $name."
	[ "$got" = "$sum" ] || fail "The update file $name does not match its checksum."
done <"$STAGE/MANIFEST"

# 2. Bring the operating system up to date. syspatch can patch itself first, so
# run it until it has nothing left (three rounds at most). A kernel patch only
# takes effect after a restart, which this script never does: it notes that one
# is pending and srvcman shows that quietly.
STEP=os
put running os "Preparing the system"
bsd_before=$(stat -f %m /bsd 2>/dev/null || echo 0)
round=0
while [ "$round" -lt 3 ]; do
	round=$((round + 1))
	log "syspatch round $round: checking"
	list=$(syspatch -c 2>>"$LOG") || fail "The system could not be checked for updates."
	if [ -z "$list" ]; then
		log "syspatch: nothing to apply"
		break
	fi
	log "syspatch: applying: $(printf '%s' "$list" | tr '\n' ' ')"
	syspatch >>"$LOG" 2>&1 || fail "The system could not be updated."
done
bsd_after=$(stat -f %m /bsd 2>/dev/null || echo 0)
if [ "$bsd_before" != "$bsd_after" ]; then
	REBOOT=1
	log "the kernel changed: a restart is pending"
fi

# 3. Unpack the update and run its install script.
STEP=install
put running install "Installing the update"
PKG="$JOBDIR/pkg"
rm -rf "$PKG"
mkdir -p "$PKG" || fail "Could not prepare the update."
for f in "$STAGE"/*; do
	name=${f##*/}
	case "$name" in
	MANIFEST | VERSION) continue ;;
	*.tar.gz | *.tgz)
		if tar -tzf "$f" 2>>"$LOG" | grep -Eq '(^|/)\.\.(/|$)|^/'; then
			fail "The update package has unsafe file names."
		fi
		tar -xzf "$f" -C "$PKG" >>"$LOG" 2>&1 || fail "The update package could not be unpacked."
		;;
	*)
		cp "$f" "$PKG/$name" || fail "Could not prepare the update."
		;;
	esac
done
if [ -f "$PKG/install.sh" ]; then
	SRC="$PKG"
else
	SRC=
	for d in "$PKG"/*/; do
		if [ -f "${d}install.sh" ]; then
			SRC=${d%/}
			break
		fi
	done
fi
[ -n "$SRC" ] || fail "The update has no install script."
log "running $SRC/install.sh"
(
	cd "$SRC" &&
		SPECZ_UPDATE_VERSION="$VERSION" SPECZ_UPDATE_JOB="$JOB" SPECZ_UPDATES_DIR="$BASE" \
			/bin/sh ./install.sh
) >>"$LOG" 2>&1 || fail "The update's install script failed."

rm -rf "$PKG" "$STAGE"
log "update $VERSION installed"
put success done "The update was installed."
DONE=1
exit 0
