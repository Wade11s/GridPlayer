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
- Unknown top-level settings and unknown per-video fields are preserved on
  load/save (they are re-emitted unchanged), so files written by the Python
  app keep their data even where this build does not use it yet.
- Writes are atomic (temp file + rename).
- The app saves position, mute, volume, rate, view mode, playback state and
  the grid subset. Bookmarks, lists, snapshots and the legacy
  `#GRIDPLAYER` text format are later increments.
- `--playlist PATH` loads at startup; File → Open/Save/Save As manage the
  document. Closing with unsaved structural changes asks Save / Discard /
  Cancel; `--autoquit` skips the prompt for automation.

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

The app restores grid mode/size, Fill/Fit, default volume/mute, window
bounds and the recent list (`app/recent`, a JSON array). Unknown keys and
sections survive a load/save. Settings are written atomically.

Not implemented yet: the full settings dialog, per-setting UI, keymap
overrides, tray/media-key options, and legacy key migrations beyond the
ones this build reads.
