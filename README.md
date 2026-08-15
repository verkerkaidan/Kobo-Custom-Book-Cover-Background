# kobo-dynamic-screensaver

A dynamic sleep screen for Kobo e-readers: your own background image, with the
current book's title, reading percentage, a progress bar, and lifetime book
counts drawn on top. Refreshes itself as you read.

**One static ARM binary. No Python, no interpreter, no runtime dependencies.**
No KOReader. No patched firmware. Nickel is untouched.

![preview](preview.png)

---

## How it works

Nickel is Kobo's reader app. It is closed source and firmware-signed — it
cannot be forked. So this sits alongside it instead:

```
Nickel  ──writes──▶  KoboReader.sqlite
                          │  (snapshot, read-only)
                          ▼
                  kobo-screensaver  ──writes──▶  .kobo/screensaver/dynamic.png
                                                          │
                        Nickel ──displays on sleep────────┘
```

Nickel already knows how to display an arbitrary PNG as the sleep screen. We
keep changing what that PNG contains. Nickel never knows anything unusual is
happening.

## What gets displayed

| Field | Source |
|---|---|
| Background | the current book's cached cover art, or a fixed image |
| Frame | an optional hand-drawn PNG composited over it |
| Title & author | `content.Title`, `content.Attribution` |
| % read | `content.___PercentRead` of the newest `ReadStatus = 1` book |
| Time read / left | `content.TimeSpentReading`; time left extrapolated from pace |
| Progress bar | same percentage |
| Books finished | `COUNT(*) WHERE ReadStatus = 2` |
| Books on device | `COUNT(*)` of downloaded, user-owned books |

"Book" means an epub that is downloaded and owned by you. Note that Kobo store
purchases have the MimeType `application/x-kobo-epub+zip`, so a filter of
`NOT LIKE '%kobo%'` excludes your entire purchased library — the filter is a
whitelist on `%epub%` instead. That also keeps out the PNG trigger icons KFMon
relies on, which Nickel indexes as content and which count as *finished books*
once tapped. The device's own eLabel leaflet is excluded by path.
| Timestamp | render time |

Each is switchable in `config.ini`.

## Compositing order

```
mat_colour                     flat fill
  └─ cover art                 drawn into the frame's window
      └─ frame PNG             alpha-composited over the whole panel
          └─ scrim             dark gradient behind the text
              └─ text + bar    always on top, never obscured by artwork
                  └─ grayscale optional, applied to everything
```

The window is not configured, it is *detected*: the bounding box of the frame's
transparent pixels (alpha < 8). Draw a hole, the cover appears in it. A frame
with no transparency falls back to drawing the cover behind it, and says so in
the log; `cover_window = x,y,w,h` overrides detection.

**Page numbers are not possible.** Kobo recomputes pagination from the current
font size and margins each time a book is opened, so `___NumPages` is `-1` for
every row and `shortcover_page` is empty — "page 45 of 320" exists only in
Nickel's memory while you read, and is never persisted. Reading time is
recorded, and `show_time_spent` / `show_time_left` are the honest substitutes.

---

## Requirements on the device

1. **KFMon** — launches the watcher when you tap its library tile. KFMon has
   no autostart-on-boot for watched actions, so you tap the tile once after
   each reboot (not after each sleep — sleeping doesn't stop it). From the
   MobileRead thread; extract
   the ZIP to the USB root (use "Extract to", don't drag files manually — the
   hidden directory structure must be preserved). Prefer the `OCP-KFMon-*.zip`
   one-click variant if it's on the thread; the other one-click packages bundle
   KOReader or Plato, which you don't want.
2. *(Optional)* **NickelMenu** — adds a "Refresh sleep screen" menu entry.
   Download its `KoboRoot.tgz` into `.kobo/` and eject; it installs on reboot.

That's it. **No Python** — that was the whole point of the Go rewrite.

## Build

Go, and nothing else:

```bash
./build.sh
```

Produces a ~7 MB statically-linked armhf binary and stages `dist/`.

The SQLite driver is `modernc.org/sqlite`, which is pure Go, so this
cross-compiles with `CGO_ENABLED=0` — no C toolchain, no
`gcc-arm-linux-gnueabihf`, no platform-specific setup. SQLite, the PNG encoder,
the TrueType rasteriser and the fonts are all linked into the binary, so the
*output* has no dependencies either.

## Install

1. Put your background at `assets/background.png`, run `./build.sh`.
2. Copy the **contents** of `dist/` to the root of the Kobo's USB drive,
   merging with the existing `.kobo` and `.adds` folders.
3. Eject and reboot once, so KFMon picks up the new watch config and Nickel
   indexes the trigger tile.
4. Open your library and tap the **kobo-screensaver** tile to start the
   watcher. Repeat that tap after any reboot. This also creates the first
   image in `.kobo/screensaver/`, which the next step needs.
5. On the device: **Settings → Energy saving and privacy → "Show book covers
   full screen" → On**. There is no setting named "custom image" — this
   misleadingly-named toggle is the one, and the `.kobo/screensaver/` folder
   overrides the cover it would otherwise draw.

See `GETTING-STARTED.md` for the long version.

## Usage

```
kobo-screensaver                 # render once and exit
kobo-screensaver -watch          # stay resident, re-render as you read
kobo-screensaver -config PATH -log PATH
```

Watch mode polls the database rather than running on a timer, so it does
essentially nothing while you're not reading. `min_regen` throttles writes to
protect the flash.

The database is in **WAL mode**, which matters: Nickel's page-turn writes go to
`KoboReader.sqlite-wal` and the main file's mtime does not move until a
checkpoint — often not until unmount. Watching the main file alone therefore
misses nearly every reading update, and the sleep screen appears frozen on an
old percentage. The poll fingerprints the mtime *and size* of the database and
both sidecars; sizes matter because the internal storage is FAT32, whose
timestamps only have two-second resolution.

---

## Notes and caveats

- **The sleep-screen setting is not named what you'd expect.** Kobo never
  advertised custom screensavers, and the control is a toggle called "Show
  book covers full screen" under Energy saving and privacy. There is no
  "custom image" option to find. The *file* location
  (`/mnt/onboard/.kobo/screensaver/`) has been stable for years, and the folder
  simply overrides whatever cover Nickel would otherwise draw — which also
  means the folder must be non-empty before the behaviour kicks in.
- **Firmware 4.45.23697 is newer than I can verify.** Confirm the current
  KFMon build supports your firmware on MobileRead before installing.
- **The binary is cross-compiled and emulator-tested, not device-tested.** It
  was verified under `qemu-arm`, producing output pixel-identical to the native
  build apart from the clock. Real hardware is the real test.
- **Colour panel.** Kaleido's colour filter array drops effective resolution and
  mutes saturation. High-contrast backgrounds with light text read best; set
  `grayscale = true` if a photo dithers badly.
- **Refresh timing — the one hard limit.** Nickel chooses the sleep image *at
  the moment it suspends*, and the device freezes every process on suspend. So
  a redraw can only ever happen while the Kobo is awake, and an update made
  right as you sleep it appears on the *next* sleep, not that one. Nothing
  short of patching Nickel changes this. Sleeping, waking and sleeping again is
  the normal way to see a change land.
- **`date_format` is a Go layout, not strftime.** `02 Jan 2006 15:04`, not
  `%d %b %Y %H:%M`.
- **This never writes to `KoboReader.sqlite`** — it snapshots the file plus any
  `-wal`/`-shm` sidecars and reads the copy, and opens even that read-only.
  Back up the real one anyway before installing anything on a Kobo.
