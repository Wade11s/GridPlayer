# Playlist persistence (.gpls v1)

`internal/playlist` reads and writes GridPlayer's JSON playlist format:

```json
{
  "format": "gridplayer-playlist",
  "version": 1,
  "videos": [
    {
      "uri": "/abs/a.mp4",
      "current_position": 1200,
      "is_muted": false,
      "volume": 0.8,
      "rate": 1,
      "cell_view_mode": "fill",
      "playback_state": "playing"
    }
  ],
  "settings": {
    "grid_state": {"mode": "auto_rows", "rows": 0, "cols": 0}
  }
}
```

Rules:

- `format` must be `gridplayer-playlist`; a missing `version` means 1; versions
  above 1 are rejected (no silent downgrade).
- Relative `uri` values resolve against the playlist's directory; `file://`
  becomes a local path; network URLs pass through.
- Unknown top-level document fields (including upstream `lists`, `bookmarks`,
  `snapshots`, and `window_state`), unknown settings, and unknown per-video
  fields are preserved on load/save. They are re-emitted unchanged, so files
  written by the Python app keep their data even where this build does not
  use it yet. Per-video IDs, titles, and unknown fields live with their cell:
  swapping or inserting cells does not move that metadata to another URI or
  erase it through document aliasing.
- Writes are atomic (temp file + rename).
- The app saves position, mute, volume, rate, view mode, playback state and
  the grid subset. Per-video state also includes `audio_delay_ms`,
  `subtitle_delay_ms`, `loop_start`, `loop_end`, `transform`, `end_action`,
  `list_end_action`, `list_fail_action`, `list_fail_wait_sec`, `crop`,
  `crop_margins`, `aspect_ratio`, `scale`, `crop_ratio`, `anchor`,
  `external_audio`, `external_subtitles`, and `grid_template`.
- External audio/subtitle file arrays resolve relative paths against the
  playlist directory, are attached in order before the first play, and are
  restored on save/reopen. Changing the restored set reloads a live cell;
  moving to another list item clears that item's external files before the
  next load. Upstream `audio_selection`/`subtitle_selection` objects are
  preserved as unknown data, but selected external track/language/device
  behavior remains a separate track-parity gap.
- Per-video list policies use the upstream 0.5.5 JSON names
  `list_end_action`, `list_fail_action`, and `list_fail_wait_sec`. Older Go
  documents using `list_end`, `list_fail`, and `list_fail_wait` are still
  accepted; canonical fields take precedence when both forms are present.
  List-end values are serialized as upstream's `start_over`, `pause`,
  `stop`, or `close`. The Go-only Next Cell action additionally retains
  `list_end:"next"` for Go while writing `list_end_action:"start_over"`
  as upstream's safe fallback. Unknown enum strings are echoed rather than
  silently replaced.
- Per-video view shape follows the upstream field names and is written
  explicitly: `aspect_ratio` (`""` is the video's own shape; `"own"` is accepted
  and treated the same), `scale` (`0` means fit; it is written only when it
  differs from a non-zero default, because upstream values start at 1.0), and
  `crop_ratio` (`""` leaves the margins
  alone; `"none"` is accepted and treated as no ratio crop). A field missing on
  load resets that cell to the current default, so a reused cell cannot leak a
  previous file's choice. `transform` and `end_action` use the upstream
  `AutoName` spellings (`rotate_90`, `rotate_180`, `rotate_270`, `loop_file`,
  `next_file`, `previous_file`, `shuffle_file`, `pause`, `stop`); the older
  short internal names are still accepted on load. `anchor` uses the upstream
  names (`center`, `top`, `bottom`, `left`, `right`, and the four corners), and
  `crop` is the upstream
  `[left, top, right, bottom]` margin array; `crop_margins` is still written for
  older Go builds and is the fallback when `crop` is absent. A missing `anchor`
  or crop on load resets that cell to the current default. The View menu's
  Aspect Ratio, Scale, Crop and Anchor items change only the active cell and
  mark the playlist dirty; Settings → Video changes the defaults and applies
  them to every cell.
- `--playlist PATH` loads at startup; File → Open/Save/Save As manage the
  document. Automatic list steps apply Start Over, Pause At Start, Stop,
  Close, or the legacy Go Next Cell action at a boundary. Manual Next and
  Previous wrap regardless of that policy. Closing with unsaved structural
  changes asks Save / Discard / Cancel; `--autoquit` skips the prompt for
  automation.

## Settings (`settings.ini`)

`internal/settings` reads/writes a QSettings-compatible INI in
`~/Library/Application Support/GridPlayer/settings.ini` (`--user-data-dir`
overrides it). Keys are written as sections:

```ini
[playlist]
grid_mode=fixed
grid_rows=2
grid_cols=2

[video_defaults]
cell_view_mode=fit
volume=0.50
muted=false

[window]
x=120
y=120
width=900
height=532
```

The app restores grid mode/size, grid line visibility, Fill/Fit, default volume/mute, window
bounds, window behavior, background pause, screensaver inhibition, language,
network resolver options, SponsorBlock categories, and the recent list
(`app/recent`, a JSON array). Unknown keys and sections survive a load/save.
Settings are written atomically.

Keymap settings include `keymap/schema=2`. On a pre-schema-2 file, values
identical to the old Go defaults are migrated once to the frozen bindings
(Space all-cell Play/Pause, Ctrl+Space active, Ctrl+Arrow short seeks,
percent-seek brackets, and corrected zoom/crop keys). Custom values and all
schema-2 values are preserved; saving writes schema 2.

Not implemented yet: tray/close-to-tray behavior, videos-per-process,
a selectable macOS driver, and legacy key migrations beyond the ones this
build reads.
