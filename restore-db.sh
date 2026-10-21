#!/usr/bin/env bash
# Restores the Kobo's library database from one of the watcher's daily backups.
#
#   ./restore-db.sh                 list the backups on the plugged-in Kobo
#   ./restore-db.sh latest          restore the newest one
#   ./restore-db.sh KoboReader-20261021-120000.sqlite
#
# Why: if a firmware update, a bad eject or a Calibre session scrambles
# KoboReader.sqlite, Nickel rebuilds the library from the files on disk and
# every finished-book flag, shelf and reading position is lost. The watcher
# keeps a few integrity-checked copies (see backup.go); this puts one back.
#
# Safe by construction: the backup is verified before anything is touched, the
# current database goes to a file next to this script first, and nothing is
# written unless the Kobo is mounted on this computer (so Nickel is not running
# against it).
set -euo pipefail

SRC="$(cd "$(dirname "$0")" && pwd)"
KOBO="${KOBO:-/Volumes/KOBOEREADER}"
BACKUPS="$KOBO/.adds/kobo-screensaver/backups"
DB="$KOBO/.kobo/KoboReader.sqlite"

command -v sqlite3 >/dev/null || { echo "error: sqlite3 not found" >&2; exit 1; }
[ -d "$KOBO/.kobo" ] || { echo "error: $KOBO is not a mounted Kobo" >&2; exit 1; }
[ -d "$BACKUPS" ] || { echo "error: no backups at $BACKUPS yet" >&2; exit 1; }

healthy() { [ "$(sqlite3 "file:$1?immutable=1" 'pragma integrity_check;' 2>&1)" = "ok" ]; }

# Newest first, by NAME: the name carries the backup's timestamp, which is what
# matters. File modification times change whenever a copy is moved around.
backups() { find "$BACKUPS" -maxdepth 1 -type f -name 'KoboReader-*.sqlite' | sort -r; }

if [ $# -eq 0 ]; then
    echo "Backups on the Kobo (newest first):"
    found=0
    while IFS= read -r f; do
        found=1
        books=$(sqlite3 "file:$f?immutable=1" "select count(*) from content where ContentType='6' and MimeType like '%epub%' and ContentID not like '%/.kobo/%'" 2>/dev/null || echo '?')
        done_=$(sqlite3 "file:$f?immutable=1" "select count(*) from content where ContentType='6' and MimeType like '%epub%' and ReadStatus=2 and ContentID not like '%/.kobo/%'" 2>/dev/null || echo '?')
        ok=$(healthy "$f" && echo ok || echo "DAMAGED")
        printf '  %-40s %5s books, %4s finished  [%s]\n' "$(basename "$f")" "$books" "$done_" "$ok"
    done < <(backups)
    [ "$found" -eq 1 ] || echo "  (none yet — the watcher takes the first one shortly after it starts)"
    echo; echo "Restore with: ./restore-db.sh latest   (or a file name from above)"
    exit 0
fi

if [ "$1" = latest ]; then
    # The newest one that is actually healthy: skipping a damaged newest backup
    # beats refusing to restore anything.
    CHOSEN=""
    while IFS= read -r f; do
        if healthy "$f"; then CHOSEN="$f"; break; fi
        echo "==> skipping damaged backup $(basename "$f")"
    done < <(backups)
    [ -n "$CHOSEN" ] || { echo "error: no healthy backup to restore" >&2; exit 1; }
else
    CHOSEN="$BACKUPS/$(basename "$1")"
fi
[ -f "$CHOSEN" ] || { echo "error: $CHOSEN not found" >&2; exit 1; }

echo "==> checking $(basename "$CHOSEN")"
healthy "$CHOSEN" || { echo "error: that backup fails its integrity check — not restoring" >&2; exit 1; }

# Nickel keeps the database in WAL mode. Set it on a scratch copy so the file
# we install looks like the one Nickel would have written.
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
cp "$CHOSEN" "$WORK/KoboReader.sqlite"
sqlite3 "$WORK/KoboReader.sqlite" "pragma journal_mode=wal;" >/dev/null
sqlite3 "$WORK/KoboReader.sqlite" "pragma wal_checkpoint(truncate);" >/dev/null
rm -f "$WORK"/KoboReader.sqlite-wal "$WORK"/KoboReader.sqlite-shm
healthy "$WORK/KoboReader.sqlite" || { echo "error: prepared copy failed its check" >&2; exit 1; }

SAFETY="$SRC/KoboReader.sqlite.before-restore-$(date +%Y%m%d-%H%M%S)"
if [ -f "$DB" ]; then
    cp "$DB" "$SAFETY"
    echo "==> current database saved to $(basename "$SAFETY")"
fi

rm -f "$DB-wal" "$DB-shm"
cp "$WORK/KoboReader.sqlite" "$DB"
sync
cmp "$WORK/KoboReader.sqlite" "$DB" || { echo "error: written copy differs" >&2; exit 1; }
find "$KOBO/.kobo" -maxdepth 1 -name '._*' -delete 2>/dev/null || true

echo "==> restored $(basename "$CHOSEN")"
echo
echo "Now eject the Kobo properly, then reboot it."
