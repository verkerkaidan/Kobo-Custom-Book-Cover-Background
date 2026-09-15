#!/usr/bin/env bash
# Copies dist/ onto a plugged-in Kobo, merging with what's already there.
#
# Never deletes anything on the device: rsync runs without --delete, so your
# books, your database and any other mods are left alone. Files we ship are
# overwritten, nothing else is touched.
set -euo pipefail

SRC="$(cd "$(dirname "$0")" && pwd)"
DIST="$SRC/dist"
KOBO="${1:-/Volumes/KOBOEREADER}"

[ -d "$DIST" ] || { echo "error: no dist/ — run ./build.sh first" >&2; exit 1; }

if [ ! -d "$KOBO" ]; then
    echo "error: Kobo not found at $KOBO" >&2
    echo "       Plug it in, wait for it to mount, and check the name under" >&2
    echo "       /Volumes. Pass the path as an argument if it differs." >&2
    exit 1
fi

# A Kobo always has this. Guards against copying onto the wrong drive.
[ -d "$KOBO/.kobo" ] || {
    echo "error: $KOBO has no .kobo folder — that isn't a Kobo drive" >&2
    exit 1; }

echo "==> target: $KOBO"
# A firmware update wipes KFMon from the rootfs. Nothing on the USB partition
# reveals that directly, but the version string changing is the tell.
[ -f "$KOBO/.kobo/version" ] && \
    echo "    firmware $(cut -d, -f3 "$KOBO/.kobo/version")  (changed since KFMon went on? see README)"

# build.sh stages the background as .dat, not .png (see its comment on why) —
# check for the file that will actually be there, or this warns on every
# single install regardless of whether a background was ever staged.
if [ ! -f "$DIST/.adds/kobo-screensaver/assets/background.dat" ]; then
    echo "    WARNING: no background image staged. The sleep screen will be"
    echo "             plain dark grey. Ctrl-C now if that isn't what you want."
    sleep 3
fi

# Back up the database before touching anything. We never write to it, but a
# copy costs seconds and this is the file you cannot replace.
BACKUP="$SRC/KoboReader.sqlite.backup-$(date +%Y%m%d-%H%M%S)"
if [ -f "$KOBO/.kobo/KoboReader.sqlite" ]; then
    cp "$KOBO/.kobo/KoboReader.sqlite" "$BACKUP"
    echo "==> database backed up to $(basename "$BACKUP")"
fi

# Keep the most recent few and delete the rest — every install leaves one of
# these, and at 2.7MB each they'd otherwise accumulate in the repo forever.
KEEP=5
old_backups=$(ls -1t "$SRC"/KoboReader.sqlite.backup-* 2>/dev/null | tail -n +$((KEEP + 1)))
if [ -n "$old_backups" ]; then
    echo "$old_backups" | xargs rm -f
    echo "==> pruned to the newest $KEEP database backups"
fi

echo "==> copying"
rsync -rv --no-perms --no-owner --no-group \
      --exclude '.DS_Store' \
      "$DIST"/ "$KOBO"/

# FAT32 drops the executable bit, but Kobo mounts the partition so that
# everything is executable anyway. Nothing to do here — noted so the missing
# chmod doesn't look like an oversight.

# macOS writes AppleDouble sidecars (._foo) for extended attributes on FAT32.
# Harmless to us, but Nickel can index the one next to our icon as a second,
# broken library tile — confusing when you're looking for the real one.
echo "==> removing macOS ._ sidecars"
find "$KOBO/.adds/kobo-screensaver" "$KOBO/icons" "$KOBO/.adds/kfmon" \
     -name '._*' -delete 2>/dev/null || true
find "$KOBO" -maxdepth 1 -name '._*' -delete 2>/dev/null || true

echo "==> verifying"
fail=0
for f in .adds/kobo-screensaver/kobo-screensaver \
         .adds/kobo-screensaver/start.sh \
         .adds/kobo-screensaver/config.ini \
         .adds/kfmon/config/screensaver.ini \
         icons/kobo-screensaver.png; do
    if [ -f "$KOBO/$f" ]; then
        echo "    ok   $f"
    else
        echo "    MISSING  $f"; fail=1
    fi
done
[ -d "$KOBO/.kobo/screensaver" ] && echo "    ok   .kobo/screensaver/" \
    || { echo "    MISSING  .kobo/screensaver/"; fail=1; }

if [ -f "$KOBO/.adds/kobo-screensaver/assets/background.dat" ]; then
    echo "    ok   background.dat"
else
    echo "    none background.dat (grey sleep screen)"
fi
for f in font.ttf font-bold.ttf; do
    [ -f "$KOBO/.adds/kobo-screensaver/assets/$f" ] && echo "    ok   $f" \
        || { echo "    MISSING  $f (render will fail)"; fail=1; }
done
# Optional: no frame simply means the cover fills the screen.
[ -f "$KOBO/.adds/kobo-screensaver/assets/frame.dat" ] \
    && echo "    ok   frame.dat" || echo "    none frame.dat (no decoration)"

[ -d "$KOBO/.adds/kfmon" ] || {
    echo "    WARNING: no .adds/kfmon on the device — KFMon isn't installed."
    echo "             Do step 2 of GETTING-STARTED.md first, or nothing runs."; }

sync
if [ "$fail" -ne 0 ]; then
    echo "==> INCOMPLETE — see MISSING above" >&2; exit 1
fi

echo
echo "==> done. Now:"
echo "    1. Eject the Kobo properly (drag to Trash / right-click Eject)"
echo "    2. Reboot it (hold power ~4s, off, then on)"
echo "    3. Tap the 'kobo-screensaver' tile in your library"
echo "    4. Settings -> Energy saving and privacy -> 'Show book covers"
echo "       full screen' = On   (there is no 'custom image' option)"
