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
DIR="/mnt/onboard/.adds/kobo-screensaver"
BIN="$DIR/kobo-screensaver"

# One instance only: tapping the icon twice should not spawn a second watcher.
#
# This used to check a saved PID with `kill -0`, which only asks "does ANY
# process with this number exist?". After a reboot the watcher is gone but
# its old, low PID gets handed to one of Nickel's own processes, so the check
# passed and the tile silently did nothing until the next reboot recycled the
# number again. Look for the actual binary in /proc instead.
running() {
    for p in /proc/[0-9]*; do
        [ "$p" = "/proc/$$" ] && continue
        # [k] keeps this grep from matching its own command line.
        if tr '\0' ' ' < "$p/cmdline" 2>/dev/null \
             | grep -q -- "$DIR/[k]obo-screensaver -config .*-watch"
        then
            return 0
        fi
    done
    return 1
}

if running; then
    exit 0
fi
rm -f "$DIR/watcher.pid"
"$BIN" -config "$DIR/config.ini" -log "$DIR/screensaver.log" -watch &
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
