# Getting Started — from a brand new Kobo

This guide assumes you have never modified a Kobo before and that the device is
still in its box. Follow it top to bottom. **Do not skip ahead and do not
reorder the steps** — a couple of them depend on the one before having finished
and rebooted.

Total time: about 45 minutes, most of it waiting for the Kobo to reboot.

---

## Before you begin: what you are actually doing

Nothing here replaces or patches your Kobo's firmware. Everything you install
lives in hidden folders on the same USB drive your books go on. If it all goes
wrong, a factory reset (or in the worst case, deleting those folders) puts you
back to stock.

You will install three things, in this order:

| # | What | Why |
|---|---|---|
| 1 | **KFMon** | A small launcher that runs a program when you tap a tile in your library. Nothing else works without it. |
| 2 | **kobo-screensaver** | This project. Draws your sleep screen. |
| 3 | **NickelMenu** *(optional)* | Adds a menu item so you can force a refresh by hand. |

> **One thing to know up front:** KFMon cannot start programs automatically at
> boot — it only reacts to you tapping a tile. So after every *reboot* you tap
> one tile in your library to start the watcher. Sleeping and waking the Kobo
> does **not** stop it, and you don't need to tap again for that; reboots are
> rare, so in practice this is a once-in-a-while thing.

"Nickel" is the name of Kobo's built-in reading software. You'll see the word a
lot in Kobo forums. We never modify it.

---

## Step 0 — Set the Kobo up normally first

Do the boring stuff before any modding:

1. Charge it.
2. Turn it on and complete the setup wizard (language, Wi-Fi, Kobo account).
3. **Let it finish any firmware update it offers.** Do this now, not later. A
   firmware update part-way through modding can wipe what you've installed.
4. Load at least two or three books and open one, read a few pages, and close
   it.

That last point matters more than it looks: the sleep screen shows your reading
progress, and until Nickel has recorded some progress there is nothing to draw.
A brand new device with no books will render a nearly empty screen and you'll
think the install failed.

Then **reboot** (hold power ~4 s, or Menu → Settings → Power off, then back on).

---

## Step 1 — Back up your database

Plug the Kobo into your computer with the USB cable. It appears as a drive
called `KOBOeReader`.

You need to see hidden files, because everything interesting starts with a dot:

- **Windows:** File Explorer → View → tick "Hidden items"
- **macOS:** press `Cmd` + `Shift` + `.` in Finder
- **Linux:** `Ctrl` + `H` in most file managers

Now copy `KOBOeReader/.kobo/KoboReader.sqlite` somewhere safe on your computer.
That single file holds your library, reading positions, bookmarks, and
highlights. This project only ever reads it, but back it up anyway — you're
about to install several things from strangers on the internet, mine included.

Eject the Kobo properly (don't just yank the cable).

---

## Step 2 — Install KFMon

KFMon watches for a file appearing and launches a program when it does. It's
what will start our screensaver watcher every time the Kobo boots.

1. Go to the KFMon thread on MobileRead:
   <https://www.mobileread.com/forums/showthread.php?t=274231>
2. Download the current package. **If you see a file named `OCP-KFMon-*.zip`,
   take that one.** The other one-click packages (`OCP-KOReader-*`,
   `OCP-Plato-*`) bundle whole alternative reading apps you said you don't
   want. Avoid the `KFMon-Uninstaller.zip` too, obviously.
3. Plug the Kobo in.
4. **Extract the ZIP directly to the root of the `KOBOeReader` drive.** Use your
   archive tool's "Extract to..." and point it at the drive. Say yes to merging
   or replacing folders.

   > **Do not open the ZIP and drag files out by hand.** The archive contains
   > hidden folders your operating system may not show you, and the directory
   > structure has to be preserved exactly. Dragging is the single most common
   > way this install fails.

   On macOS the reliable way is one terminal command, which cannot get the
   structure wrong:

   ```bash
   unzip -o ~/Downloads/KFMon-*.zip -d /Volumes/KOBOeReader
   ```

5. **Remove the example watches you don't need.** KFMon ships ready-made
   configs for two other apps, KOReader and Plato, along with their icons. It
   does *not* ship the apps themselves, so those icons appear in your library
   and do nothing at all when tapped — confusing later, when you're trying to
   work out whether *our* icon works. Delete them:

   ```bash
   rm -f /Volumes/KOBOeReader/.adds/kfmon/config/plato.ini \
         /Volumes/KOBOeReader/.adds/kfmon/config/koreader.ini \
         /Volumes/KOBOeReader/icons/plato.png \
         /Volumes/KOBOeReader/koreader.png
   ```

   **Keep `kfmon.png` in the drive's root.** That one is KFMon's own log
   viewer: tapping it prints KFMon's recent log to the screen, which is the
   best troubleshooting tool you have if anything goes wrong later.

6. Eject the Kobo safely and unplug it.
7. The Kobo will process the new files and **reboot itself**. This can take a
   minute or two and the screen may go blank or show a progress bar. Let it
   finish. Don't press anything.

**How to tell it worked:** the `.kobo/KoboRoot.tgz` file should have vanished
from the drive — the Kobo deletes it once it has installed it. That's the
clearest signal.

Better still, confirm KFMon is actually *running*: find the **kfmon** icon in
your library and tap it. It should print a few lines of KFMon's log on screen.
If you see that log, KFMon is alive and watching, and the rest of this guide
will work.

If `KoboRoot.tgz` is still sitting there untouched, the Kobo never installed
it — repeat step 2, and re-read the warning about dragging files.

---

## Step 3 — Install the screensaver

### 3a. Choose your background image

Any photo or picture you like. It gets automatically scaled and centre-cropped
to your screen, so the exact size doesn't matter, but:

- Landscape photos will get their sides cropped off, since the screen is tall.
  A portrait-orientation image works better.
- Text is drawn along the bottom in white with a dark gradient behind it, so
  busy or bright bottoms are fine, but a calm lower third looks best.
- Your Kobo Colour's screen mutes colours noticeably. High-contrast images
  survive this better than subtle pastel ones.

Save it as `background.png`.

> **Watch out for WebP.** Images saved from a web browser are very often WebP
> files even when the name ends in `.png` — right-click → Save Image As
> frequently produces one. The program reads PNG and JPEG only, and a WebP
> silently falls back to the plain grey background, which looks exactly like a
> failed install. Check what you actually have with:
>
> ```bash
> file assets/background.png
> ```
>
> If that says "Web/P" rather than "PNG image data", convert it first — macOS
> Preview will do it via File → Export, choosing PNG as the format.

### 3b. Get the files ready

A `dist/` folder has already been generated for you in the project directory.
It contains the compiled program and every config file, laid out exactly as it
needs to sit on the Kobo. **You do not need Go, a compiler, or any programming
tools.**

The only thing missing from it is your picture. Copy your image in:

```bash
cp /path/to/your/image.png assets/background.png
./build.sh
```

`build.sh` re-stages `dist/` with your image inside it. It does not compile
anything unless you have an ARM cross-compiler installed — without one it just
reuses the prebuilt binary, which is what you want.

> If you'd rather not touch a terminal at all, you can skip the script and drop
> your image straight into `dist/.adds/kobo-screensaver/assets/background.png`
> by hand, creating the `assets` folder if it isn't there. Same result.

**No image?** It still works — you'll get white text on a plain dark grey
background. You can add a picture later without reinstalling anything.

### 3c. Copy it across

Plug the Kobo in. Copy the **contents** of `dist/` into the root of the
`KOBOeReader` drive — that is, the `.adds`, `.kobo` and `icons` folders
*inside* `dist`, not the `dist` folder itself.

When your computer asks whether to merge folders or replace them, choose
**merge / keep both**. You are adding to existing folders, not replacing them.
Replacing `.kobo` would be bad.

Afterwards, verify these exist on the drive:

```
KOBOeReader/.adds/kobo-screensaver/kobo-screensaver     <- the program
KOBOeReader/.adds/kobo-screensaver/start.sh
KOBOeReader/.adds/kobo-screensaver/config.ini
KOBOeReader/.adds/kobo-screensaver/assets/background.png
KOBOeReader/.adds/kfmon/config/screensaver.ini
KOBOeReader/icons/kobo-screensaver.png                  <- the tile you'll tap
KOBOeReader/.kobo/screensaver/                          <- empty folder, correct
```

That `icons/kobo-screensaver.png` is deliberately in a *visible* folder, not a
hidden one. KFMon can only watch files that Nickel has indexed into your
library, and Nickel skips hidden folders. Don't move it into `.adds`.

Eject safely, unplug, then **reboot the Kobo** (hold the power button ~4 s,
power off, power on). KFMon only reads its config at boot, so this reboot is
required.

### 3d. Start the watcher

After the reboot, Nickel will have indexed the new tile.

1. Go to your **Home** screen or **My Books**. You'll see a new item that looks
   like a small dark tile with a progress bar on it — it may be listed as
   "kobo-screensaver". If you don't see it, it's usually sorted by date added,
   so check the most recent items, and give the Kobo a minute to finish
   indexing.
2. **Tap it once.** KFMon ships with on-screen notifications enabled, so you
   should get a brief popup telling you the action started successfully. That
   popup is your confirmation.
3. The screen may also flicker, or a blank page may open. That's normal — back
   out of it. If you got the popup, the watcher is running.

That tile is a launcher, not a book. Tapping it again is harmless — the script
checks whether the watcher is already running and won't start a second one.

**You need to tap it again after every reboot**, but not after sleeping. If
you install NickelMenu in step 6, you get a menu entry that does the same thing
without hunting for the tile.

---

## Step 4 — Tell Kobo to use the image

**There is no setting called "custom image" or "screensaver".** Don't go
looking for one — this is the single most confusing part of the whole install.
Kobo never advertised this feature, and the toggle that controls it is named
after something else entirely.

On the device:

**More (or the hamburger menu) → Settings → Energy saving and privacy →
turn ON "Show book covers full screen"**

That's it. The name is misleading: with it on, Nickel intends to show your
current book's cover at sleep — but if `.kobo/screensaver/` contains an image,
that image is used instead. Our program keeps a file there, so our image wins.

If you also see a toggle along the lines of **"Show current read"** or a sleep
screen on/off switch in the same panel, turn that on too. It controls whether
anything at all is drawn at sleep.

> **Order matters.** `.kobo/screensaver/` has to actually contain an image
> before this works, and it is empty until the watcher runs for the first time.
> If you haven't done step 3d yet — tapping the tile — do it before this step,
> or there will be nothing for Nickel to find.

---

## Step 5 — Check that it works

1. Put the Kobo to sleep: press the power button briefly (a short press sleeps,
   a long press powers off).
2. Look at the screen. You should see your image with your current book's
   title, author, percentage, a progress bar, and the counts along the bottom.

**Percentage doesn't match the page numbers in the reader?** That's expected,
not a fault. Kobo's percentage measures your position in the book's *text*,
while page numbers are worked out from your current font size. Front matter —
title page, copyright, contents, dedication — eats printed pages while barely
advancing through the text, so early in a book the percentage reads lower than
pages-so-far ÷ total would suggest. The gap closes as you get further in, and
the reader's own percentage will agree with the sleep screen.

**Nothing changed?** Read a few pages of a book first, then wait a minute, then
sleep it again. Two things are worth understanding here:

- The watcher only redraws when something on screen would actually change, and
  no more often than once every 45 seconds.
- Nickel picks the sleep image **at the moment it goes to sleep**, and the
  Kobo freezes every running program while suspended. So the redraw has to
  happen while the device is awake, and a change made just as you sleep it
  appears on the *next* sleep, not that one. Sleeping, waking, and sleeping
  again is the normal way to see an update. This is a limit of how Nickel
  works, not something the settings can tune away.

**Still nothing?** Plug in and open
`KOBOeReader/.adds/kobo-screensaver/screensaver.log` in a text editor. It is
plain text and will say what went wrong. Common causes:

| Log says | Fix |
|---|---|
| `database not found` | Path in `config.ini` is wrong, or the Kobo was plugged in (USB mode unmounts the database) |
| `no usable font found` | Shouldn't happen now — fonts ship in `assets/`. It means `assets/font.ttf` didn't get copied across. Re-run `install-to-kobo.sh` |
| Log file doesn't exist at all | The watcher was never started. Nine times out of ten you haven't tapped the tile (step 3d), or you rebooted since you last tapped it |
| Renders, but bottom text is cut off | Screen size was mis-detected; set `width` and `height` in `config.ini` explicitly |

**Check KFMon's side of it too.** Tap the **kfmon** icon in your library and it
prints its own log to the screen. That tells you whether KFMon registered our
watch at all. A line complaining that it cannot find
`/mnt/onboard/icons/kobo-screensaver.png` means the icon didn't get copied, or
Nickel hasn't indexed it yet.

**No log file and no tile in your library either?** Then Nickel hasn't indexed
`icons/kobo-screensaver.png`. Check the file is really at
`KOBOeReader/icons/kobo-screensaver.png` (not inside `.adds`), then reboot the
Kobo again — indexing happens on boot and after USB eject.

**Screen is plain dark grey with correct text on it?** That's the "no
background image found" fallback, and it means everything works except the
picture. Check that your image is at
`KOBOeReader/.adds/kobo-screensaver/assets/background.png`, spelled exactly
that way, and that it really is a PNG or JPEG.

---

## Step 6 *(optional)* — NickelMenu

Worth it for two reasons: a "refresh right now" button instead of waiting for
the automatic poll, and a menu entry to restart the watcher after a reboot so
you don't have to find the tile in your library.

1. Download `KoboRoot.tgz` from <https://pgaskin.net/NickelMenu/>
2. Plug the Kobo in and copy that file into `KOBOeReader/.kobo/`
3. Eject. The Kobo reboots and installs it.
4. A **NickelMenu** entry appears in the top-left main menu, and because the
   screensaver package already shipped `.adds/nm/screensaver`, you'll also get
   two entries of ours: **"Refresh sleep screen"** (redraw once, right now)
   and **"Start sleep-screen watcher"** (the same thing tapping the tile does,
   for after a reboot).

> One quirk: NickelMenu has a failsafe that disables it if you reboot within
> about 20 seconds of it first starting. If the menu doesn't appear, reboot
> normally and give it a minute.

---

## Adjusting things later

Everything is in `KOBOeReader/.adds/kobo-screensaver/config.ini`. Plug in, edit
it in any plain text editor, eject. The watcher notices the file changed and
redraws within about a minute — no reboot, no tapping the tile. (The polling
timings themselves are the exception: those are read once at startup.)

Useful knobs:

- `position = top` moves the text block to the top of the screen
- `grayscale = true` if your image dithers into a mess on the colour panel
- `text_offset = 40` nudges the whole text block down (negative moves it up),
  for when a frame's ornaments sit where the text wants to be
- `show_library_count = false` etc. to hide individual lines
- `show_time_spent = true` adds how long you've spent on the current book, e.g.
  `13% read · 3h 1m`
- `show_time_left = true` adds an estimate of how much reading is left, e.g.
  `13% read · ~20h 16m left`, worked out from your pace so far

> **Page numbers aren't available**, unfortunately. Kobo works out pages from
> your font size and margins each time you open a book, so they only exist
> while you're reading and never get saved anywhere the sleep screen can read
> them. The two time settings above are the nearest real equivalent.
- `text_colour = 20,20,20` plus `scrim = false` for dark text on a light image
- `date_format` uses Go's odd reference date. `02` = day, `01` = month,
  `2006` = year, `15:04` = time. So `02 Jan 2006` gives "15 Aug 2026".

## The background follows the book

By default (`cover_background = true`) the sleep screen uses **the cover of
whatever book you're currently reading**, automatically. Start a new book and
the wallpaper changes by itself — no cable, no file swapping.

This works because Nickel already caches every book's cover art on the device,
at close to full screen size. The program looks up the cover belonging to the
book you have open and draws the stats over it.

The fixed image below is the fallback. It gets used when `cover_background` is
`false`, when no book is currently open, or when a cover hasn't been cached yet
(a book you've downloaded but never opened).

> **Tip:** covers usually show the title and author already, so the overlay can
> read a little redundantly. Setting `show_title = false` in `config.ini`
> leaves just the percentage, progress bar and counts. It's a one-line change
> on the device — no rebuild.

## Drawing your own frame

You can draw a pattern that appears **around** the cover — a border, corner
flourishes, whatever you like. It works by drawing a full-screen picture with a
**see-through hole** in the middle. The cover goes in the hole; your artwork
surrounds it.

You never have to measure or configure anything: the program finds the hole by
looking for the transparent part of your drawing.

There are two generated examples. The one installed is `assets/frame.png`,
semi-transparent vines laid over the whole cover — it has no hole at all, so
`config.ini` sets `cover_window = full` to put the cover behind the entire
panel. The other is `assets/frame-artdeco.png`, a dark art-deco border
with a hole (use `cover_window = auto` with that one, and `text_offset = 32`
to clear its inner rules). Replace either with your own whenever you like, or
delete `.adds/kobo-screensaver/assets/frame.dat` from the Kobo to go back to a
full-screen cover with no decoration.

To regenerate either (or tweak colours, alpha and shapes, which are all near
the top of each file):

```bash
go run ./tools/mkvines assets/frame.png
go run ./tools/mkframe assets/frame-artdeco.png
```

### How to draw one

1. Open `assets/frame-template.png` in your drawing app. It's a guide showing
   the screen size, a suggested hole, and the strip where the stats get drawn.
2. Add a new layer and draw your pattern on it.
3. Delete the template layer.
4. **Erase the middle to fully transparent** — this is the step that matters.
   The checkerboard area on the template shows roughly where.
5. Export as a PNG, at **1072 x 1448**, with transparency kept.
6. Save it as `assets/frame.png`, then run:

```bash
./build.sh && ./install-to-kobo.sh
```

Or, to change it directly on the Kobo without rebuilding: plug in, drop your PNG
at `.adds/kobo-screensaver/assets/frame.dat` (that exact name, `.dat` not
`.png`), and eject. The screen redraws by itself within a minute — you don't
need to reboot or tap anything.

### Seeing it before it goes on the Kobo

Ejecting and waiting to check every change is painful. With the Kobo plugged in:

```bash
./preview.sh
```

That renders the real thing — your actual current book, its real cover, your
frame — and opens the image. Seconds per iteration instead of minutes. You can
also preview a specific file without installing it:

```bash
./preview.sh ~/Desktop/my-new-frame.png
```

### Things worth knowing before you spend hours on it

- **Keep detail out of the bottom fifth.** The title, percentage, bar and counts
  are drawn on top of everything, over a dark gradient. Anything intricate down
  there gets covered.
- **The screen mutes colour.** The Kaleido panel drops saturation and effective
  resolution noticeably. Bold shapes and strong contrast survive; fine pencil
  detail and pastel shades mostly don't.
- **Light artwork plus white text is hard to read.** If your pattern is pale at
  the bottom, either darken it there or switch the text to dark
  (`text_colour = 40,40,40`, `scrim = false` in `config.ini`).
- **If you forget to erase the hole**, the cover ends up hidden behind your
  drawing. The log says so in as many words, and `build.sh` warns you when the
  file has no transparency at all.

### Settings

In `config.ini`:

- `frame` — path to the artwork. Leave blank for no frame at all.
- `cover_window` — `auto` finds the hole by itself. `full` puts the cover
  behind the whole panel, for overlays with no hole. Set `x,y,w,h` to place
  the cover somewhere specific instead.
- `cover_fit` — `cover` fills the hole, cropping the cover's edges so there are
  no gaps. `contain` shows the whole cover with `mat_colour` around it.
- `mat_colour` — what shows behind the cover, and anywhere your artwork is
  see-through outside the hole.

## Changing the fallback picture

You do **not** need this project, a rebuild, or a reboot to change the
background. Once installed, it's a file swap on the device:

1. Plug the Kobo in.
2. Replace `KOBOeReader/.adds/kobo-screensaver/assets/background.dat` with the
   new image, **renaming it to `background.dat`**.
3. Eject.

> **Why `.dat` and not `.png`?** Kobo indexes any image it finds — including
> inside hidden folders — into your library as though it were a book. Left as
> `background.png`, your wallpaper shows up on your Home screen as a phantom
> book called "background". The `.dat` name hides it. The program doesn't care
> about the extension; it looks at the file's actual contents, so a PNG or JPEG
> named `background.dat` works perfectly.

The next redraw picks it up. If you want to see it immediately rather than
waiting, use the NickelMenu **"Refresh sleep screen"** entry, then sleep the
device.

Two things to check when handing this to someone else:

- **The filename must stay `background.dat`**, whatever the picture is.
- **It must really be a PNG or JPEG.** Images saved from a browser are very
  often WebP with a `.png` name, and the program can't read those — it falls
  back to plain grey, which looks like a broken install. If a new image gives
  you a grey screen, this is almost certainly why. macOS Preview converts it:
  open the image, File → Export, set Format to PNG.

The current setup — white text over a dark gradient at the bottom — was chosen
so it stays readable over *any* image, light or dark, without retuning
anything. That's why it isn't colour-matched to the Cubone picture.

---

## Undoing it all

In reverse order of installation:

1. **NickelMenu:** create an empty file named `uninstall` in
   `KOBOeReader/.adds/nm/`, then eject.
2. **This project:** delete `KOBOeReader/.adds/kobo-screensaver/`,
   `KOBOeReader/.adds/kfmon/config/screensaver.ini` and
   `KOBOeReader/icons/kobo-screensaver.png`. Set the sleep screen back
   to "Book cover" in Settings.
3. **KFMon:** download `KFMon-Uninstaller.zip` from the same MobileRead thread
   and extract it to the drive root, same way you installed it.

A factory reset from the device's own settings also clears everything, at the
cost of your library and reading positions.

---

## Honest limitations

- I have not tested any of this on a physical Kobo. The rendering has been
  verified by running the program against a synthetic copy of a Kobo database
  and checking the resulting image — the database queries, layout, progress bar
  and counts are all confirmed correct. What that test does *not* cover is the
  ARM binary itself running on the device, and the install procedure, which is
  assembled from the official KFMon and NickelMenu documentation. Real hardware
  is the real test, and something may surprise you.
- Your firmware version is newer than my knowledge, so menu locations in
  particular may have moved. Before installing, it's worth checking the
  MobileRead KFMon thread for recent posts confirming it works on 4.45.x —
  those threads are where breakage gets reported first.
- Modding a Kobo is generally low-risk and widely done, but it is unsupported
  by Kobo. Your backup from step 1 is your safety net. Keep it.

### The sleep screen stopped updating, and the tile does nothing

Check `.kobo/version` on the USB drive: the third field is the firmware.
If it has changed since KFMon was installed, a Wi-Fi firmware update has
wiped KFMon from the internal filesystem, and the tile is now just a picture.
Copy `.adds/kfmon/KoboRoot.tgz.reinstall` (a stashed copy of the KFMon
installer) to `.kobo/KoboRoot.tgz`, eject, reboot, tap the tile.
`screensaver.log` will show a `start.sh: launching watcher` line once it
works again; `ps-at-tap.txt` next to it is a snapshot of what
was running when the tile was tapped, for when it doesn't.
