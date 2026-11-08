#!/usr/bin/env python3
"""Generate deterministic non-private PoC fixtures (testsrc + timecode + tone)."""
import hashlib
import json
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent / "testdata" / "fixtures"
MANIFEST = ROOT / "manifest.json"

PLONS = [
    # name, seconds, size, freq (Hz), label
    ("plane-a.mp4", 10, "640x360", 440, "A"),
    ("plane-b.mp4", 10, "640x360", 550, "B"),
    ("plane-c.mp4", 10, "640x360", 660, "C"),
    ("plane-d.mp4", 10, "640x360", 770, "D"),
]

FONT_PATH = "/Library/Fonts/Arial Unicode.ttf"
W, H, FPS = 640, 360, 25

def render_frame(idx: int, total: int, label: str):
    from PIL import Image, ImageDraw, ImageFont
    img = Image.new("RGB", (W, H))
    d = ImageDraw.Draw(img)
    # shifting color bars (motion + identity)
    bars = ["RED", "GREEN", "BLUE", "YELLOW", "MAGENTA", "CYAN"]
    phase = idx % (W * 2)
    bw = 80
    for i in range(-1, 10):
        x0 = (i * bw + phase) % (W + bw) - bw
        d.rectangle([x0, 0, x0 + bw, H], fill=bars[i % len(bars)])
    # moving ball + diagonal
    bx = int(W * (0.5 + 0.45 * __import__("math").sin(idx * 0.05)))
    by = int(H * (0.5 + 0.4 * __import__("math").cos(idx * 0.07)))
    d.ellipse([bx - 30, by - 30, bx + 30, by + 30], fill="WHITE", outline="BLACK", width=4)
    d.line([(0, (idx * 2) % H), (W, H - (idx * 2) % H)], fill="BLACK", width=3)
    try:
        f_big = ImageFont.truetype(FONT_PATH, 64)
        f_small = ImageFont.truetype(FONT_PATH, 28)
    except Exception:
        f_big = ImageFont.load_default()
        f_small = f_big
    sec = idx / FPS
    tc = f"{sec // 3600:02.0f}:{(sec % 3600) // 60:02.0f}:{sec % 60:02.0f}.{int((sec % 1) * FPS):02d}"
    text = f"{label} {idx:04d} {tc}"
    d.rectangle([10, 10, 480, 110], fill=(0, 0, 0))
    d.text((18, 18), text, fill="WHITE", font=f_big)
    return img

def run(*args):
    subprocess.run(args, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)

FONT = "/Library/Fonts/Arial Unicode.ttf"

def make_one(root: Path, name: str, secs: int, size: str, freq: int, label: str):
    out = root / name
    import subprocess
    cmd = ["ffmpeg", "-nostdin", "-loglevel", "error", "-y",
           "-f", "rawvideo", "-pix_fmt", "rgb24", "-s", f"{W}x{H}", "-r", str(FPS), "-i", "-",
           "-f", "lavfi", "-i", f"sine=frequency={freq}:duration={secs}",
           "-map", "0:v", "-map", "1:a", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-profile:v", "baseline",
           "-c:a", "aac", "-b:a", "128k", "-shortest", str(out)]
    p = subprocess.Popen(cmd, stdin=subprocess.PIPE)
    n = secs * FPS
    for idx in range(n):
        p.stdin.write(render_frame(idx, n, label).tobytes())
    p.stdin.close()
    rc = p.wait()
    if rc != 0:
        raise RuntimeError(f"ffmpeg rc={rc} for {out}")
    return out

def main():
    import shutil
    ROOT.mkdir(parents=True, exist_ok=True)
    files = []
    for name, secs, size, freq, label in PLONS:
        out = make_one(ROOT, name, secs, size, freq, label)
        data = out.read_bytes()
        files.append({
            "name": name,
            "sha256": hashlib.sha256(data).hexdigest(),
            "bytes": len(data),
            "secs": secs, "size": size, "freq": freq, "label": label,
        })
    manifest = {"generator": "scripts/make_fixtures.py", "ffmpeg": "9.0.2", "planes": files}
    MANIFEST.write_text(json.dumps(manifest, indent=2) + "\n")
    print(f"fixtures -> {ROOT}")
    for f in files:
        print(f"  {f['name']} sha256={f['sha256'][:16]}... {f['bytes']}B")

if __name__ == "__main__":
    main()
