#!/usr/bin/env bash
# Lays out the deploy tree in dist/, ready to copy onto the Kobo's USB drive.
#
# Needs Go and nothing else. The SQLite driver is modernc.org/sqlite, which is
# pure Go, so this cross-compiles to ARM with CGO off — no C toolchain, no
# platform-specific setup. (The older cgo driver, go-sqlite3, required
# gcc-arm-linux-gnueabihf; that requirement is gone.)
set -euo pipefail

SRC="$(cd "$(dirname "$0")" && pwd)"
OUT="$SRC/dist"
BIN="$SRC/kobo-screensaver"

# --- build ------------------------------------------------------------------

echo "==> cross-compiling for armhf (pure Go, CGO off)"
cd "$SRC"
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 \
    go build -trimpath -ldflags '-s -w' -o "$BIN" .

# With CGO off the result is always static, but check anyway: a dynamically
# linked binary would need libc on the device and fail at launch.
case "$(file -b "$BIN")" in
    *ARM*statically\ linked*) ;;
    *) echo "error: $BIN is not a static ARM binary:" >&2
       file "$BIN" >&2; exit 1 ;;
esac
echo "    $(du -h "$BIN" | cut -f1) static ARM binary"

# --- stage the deploy tree --------------------------------------------------

echo "==> staging deploy tree"
rm -rf "$OUT"
mkdir -p "$OUT/.adds/kobo-screensaver/assets" "$OUT/.kobo/screensaver" \
         "$OUT/.adds/kfmon/config" "$OUT/.adds/nm" "$OUT/icons"

cp "$BIN" "$SRC/config.ini" "$OUT/.adds/kobo-screensaver/"
chmod +x "$OUT/.adds/kobo-screensaver/kobo-screensaver"

# Staged as .dat, not .png, on purpose: Nickel indexes images even inside
# hidden folders (FW >= 4.17), so a background.png turns up in the library as a
# phantom book called "background". The decoder sniffs file contents and
# ignores the extension, so this costs nothing.
if [ -f "$SRC/assets/background.png" ]; then
    cp "$SRC/assets/background.png" \
       "$OUT/.adds/kobo-screensaver/assets/background.dat"
else
    echo "    NOTE: assets/background.png missing — the sleep screen will be"
    echo "          plain dark grey until you add one. See GETTING-STARTED.md."
fi

# The hand-drawn frame, same .dat treatment. Optional: no frame.png just means
# the cover fills the screen as before.
if [ -f "$SRC/assets/frame.png" ]; then
    cp "$SRC/assets/frame.png" "$OUT/.adds/kobo-screensaver/assets/frame.dat"
    if command -v sips >/dev/null && \
       ! sips -g hasAlpha "$SRC/assets/frame.png" 2>/dev/null | grep -q "hasAlpha: yes"
    then
        echo "    WARNING: assets/frame.png has no transparency. The cover will"
        echo "             be hidden behind it. Erase the middle to see-through."
    fi
    echo "    frame staged"
fi
# Fonts are bundled deliberately: Kobo firmware does not reliably ship Georgia
# at /usr/local/Kobo/fonts/, and a missing font is fatal to rendering.
for f in font.ttf font-bold.ttf; do
    [ -f "$SRC/assets/$f" ] && cp "$SRC/assets/$f" \
        "$OUT/.adds/kobo-screensaver/assets/" || true
done

# KFMon triggers on a PNG that Nickel has indexed as a library tile, so this
# lives in a visible folder, NOT under .adds — tapping the tile is what starts
# the watcher. Nickel skips dotfolders on older firmware.
cp "$SRC/assets/trigger.png" "$OUT/icons/kobo-screensaver.png"

# KFMon launches a single command; this wrapper starts watch mode.
cat > "$OUT/.adds/kobo-screensaver/start.sh" <<'EOF'
#!/bin/sh
# Tapping the library tile means "(re)start the watcher".
#
# It used to mean "start unless already running", guarded by a saved PID and
# `kill -0`. That guard passed on the wrong process after a reboot (the low
# PID is recycled by Nickel), and would equally step aside for a watcher that
# is alive but wedged. So: stop whatever is there, then launch a fresh one
# under a tiny supervisor that relaunches it if it ever exits. Everything the
# script does is written to the log, so a tap that achieves nothing at least
# says so.
DIR="/mnt/onboard/.adds/kobo-screensaver"
BIN="$DIR/kobo-screensaver"
LOG="$DIR/screensaver.log"
TAG="kobo-screensaver-supervisor"

# KFMon's own udev rule admits it runs "early at boot... onboard *might* be
# mounted at that point", and Nickel's boot-time library rescan can retrigger
# this tile's inotify watch several times before the partition is actually
# ready. There is nowhere safe to log a failure yet -- the log lives on the
# same not-yet-mounted partition -- so previously each premature attempt fell
# through to the "$1" exec failing, which busybox's sh then tried to
# re-interpret as a shell script, dumping raw binary garbage into the log the
# moment it *did* become writable. Wait for the actual binary to exist and be
# runnable first, instead.
i=0
while [ ! -x "$BIN" ]; do
    i=$((i + 1))
    # ~15s, then give up -- but non-zero, not 0: KFMon shows an on-screen
    # error for a non-zero exit regardless of notification settings, and a
    # binary that's still missing after 15s is more likely a genuinely
    # broken install than a slow mount. A quick race resolves in a couple of
    # seconds either way, so this only ever fires for the case worth seeing.
    [ "$i" -ge 15 ] && exit 1
    sleep 1
done

say() { echo "$(date '+%Y/%m/%d %H:%M:%S') start.sh: $*" >> "$LOG"; }

# What was running when the tile was tapped, before we touch anything. The
# one question a silent log can't answer from the outside.
ps w > "$DIR/ps-at-tap.txt" 2>&1

# The supervisor goes first, or it would respawn the watcher we stop next.
for pat in "$TAG" "$BIN -config"; do
    for p in /proc/[0-9]*; do
        pid="${p#/proc/}"
        [ "$pid" = "$$" ] && continue
        cmd="$(tr '\0' ' ' < "$p/cmdline" 2>/dev/null)"
        case "$cmd" in
            *"$pat"*) kill "$pid" 2>/dev/null && say "stopped pid $pid" ;;
        esac
    done
done
rm -f "$DIR/watcher.pid"

say "launching watcher"
# $0 of the inner shell is the tag, which is how the loop above finds it.
#
# The watcher's stderr goes to the log too — a Go panic prints there, and
# KFMon's stdout is nowhere anyone looks. It is piped through a read loop
# rather than redirected once, because a descriptor opened before a USB
# session points at a dead mount afterwards and everything written to it
# is lost. Appending per line reopens the file every time.
#
# The relaunch delay doubles on every exit, up to ten minutes, and resets
# after a run that lasted at least an hour: a watcher that dies instantly
# (half-copied binary, missing font) must not write to flash every 30s
# forever.
sh -c '
    delay=30
    while :; do
        began=$(date +%s)
        "$1" -config "$2/config.ini" -log "$2/screensaver.log" -watch 2>&1 \
            | while IFS= read -r line; do
                  echo "$(date "+%Y/%m/%d %H:%M:%S") stderr: $line" >> "$2/screensaver.log"
              done
        [ $(( $(date +%s) - began )) -ge 3600 ] && delay=30
        echo "$(date "+%Y/%m/%d %H:%M:%S") supervisor: watcher exited, relaunching in ${delay}s" >> "$2/screensaver.log"
        sleep "$delay"
        [ "$delay" -lt 600 ] && delay=$(( delay * 2 ))
    done
' "$TAG" "$BIN" "$DIR" </dev/null >/dev/null 2>&1 &
echo $! > "$DIR/watcher.pid"
EOF
chmod +x "$OUT/.adds/kobo-screensaver/start.sh"

cat > "$OUT/.adds/kfmon/config/screensaver.ini" <<'EOF'
[watch]
filename = /mnt/onboard/icons/kobo-screensaver.png
action = /mnt/onboard/.adds/kobo-screensaver/start.sh
label = Start sleep-screen watcher
hidden = 0
EOF

# NickelMenu: start the watcher, or force one immediate re-render.
cat > "$OUT/.adds/nm/screensaver" <<'EOF'
menu_item :main :Refresh sleep screen :cmd_spawn :quiet:/mnt/onboard/.adds/kobo-screensaver/kobo-screensaver -config /mnt/onboard/.adds/kobo-screensaver/config.ini -log /mnt/onboard/.adds/kobo-screensaver/screensaver.log
menu_item :main :Start sleep-screen watcher :cmd_spawn :quiet:/mnt/onboard/.adds/kobo-screensaver/start.sh
EOF

echo "==> built $OUT"
echo "    copy the CONTENTS of dist/ to the root of the Kobo USB drive"
