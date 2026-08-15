#!/usr/bin/env bash
# Renders the sleep screen on this computer and opens it, so you can iterate on
# artwork without ejecting and rebooting the Kobo every time.
#
#   ./preview.sh                 use the real database + covers if the Kobo is
#                                plugged in, otherwise the last snapshot
#   ./preview.sh some-frame.png  preview a specific frame file
#
# Nothing here touches the Kobo: the database is copied, never written.
set -euo pipefail

SRC="$(cd "$(dirname "$0")" && pwd)"
WORK="$SRC/.preview"
KOBO="${KOBO:-/Volumes/KOBOEREADER}"
FRAME="${1:-$SRC/assets/frame.png}"

mkdir -p "$WORK"

# --- database ---------------------------------------------------------------

if [ -f "$KOBO/.kobo/KoboReader.sqlite" ]; then
    cp "$KOBO/.kobo/KoboReader.sqlite" "$WORK/KoboReader.sqlite"
    for x in -wal -shm; do
        cp "$KOBO/.kobo/KoboReader.sqlite$x" "$WORK/KoboReader.sqlite$x" 2>/dev/null || true
    done
    echo "==> using the live database"
elif [ -f "$WORK/KoboReader.sqlite" ]; then
    echo "==> Kobo not plugged in; using the last snapshot"
else
    echo "error: plug the Kobo in once so there is a database to preview against" >&2
    exit 1
fi

# --- covers -----------------------------------------------------------------

COVERS=""
if [ -d "$KOBO/.kobo-images" ]; then
    COVERS="$KOBO/.kobo-images"
    echo "==> using the real cover art"
else
    echo "==> no cover cache reachable; falling back to the background image"
fi

# --- render -----------------------------------------------------------------

FRAME_LINE=""
if [ -f "$FRAME" ]; then
    FRAME_LINE="frame = $FRAME"
    echo "==> frame: $FRAME"
else
    echo "==> no frame (put one at assets/frame.png, or pass a path)"
fi

cat > "$WORK/preview.ini" <<EOF
[screensaver]
db_path          = $WORK/KoboReader.sqlite
output           = $WORK/preview.png
cover_background = $([ -n "$COVERS" ] && echo true || echo false)
kobo_images      = $COVERS
background       = $SRC/assets/background.png
$FRAME_LINE
font             = $SRC/assets/font.ttf
font_bold        = $SRC/assets/font-bold.ttf
width            = 1072
height           = 1448
EOF

# Carry across the look settings from the real config so the preview matches
# what the device will draw.
grep -E '^(position|text_colour|shadow|scrim|margin|title_size|body_size|small_size|show_|date_format|grayscale|cover_window|cover_fit|mat_colour)' \
    "$SRC/config.ini" >> "$WORK/preview.ini" || true

go run "$SRC" -config "$WORK/preview.ini"

echo "==> $WORK/preview.png"
command -v open >/dev/null && open "$WORK/preview.png" || true
