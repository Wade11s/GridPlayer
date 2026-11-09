package medialist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestM3U(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "list.m3u", "#EXTM3U\n#EXTINF:1,A\na.mp4\nhttps://x/y.m3u8\n/abs/b.mkv\n")
	got, err := Expand(p, false, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "a.mp4"), "https://x/y.m3u8", "/abs/b.mkv"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v", got)
	}
}

func TestPLSAndXSPF(t *testing.T) {
	dir := t.TempDir()
	pls := write(t, dir, "list.pls", "[playlist]\nFile1=a.mp4\nFile2=b.mp4\nNumberOfEntries=2\n")
	got, err := Expand(pls, false, 10)
	if err != nil || len(got) != 2 || got[1] != filepath.Join(dir, "b.mp4") {
		t.Fatalf("pls %v %v", got, err)
	}
	xspf := write(t, dir, "list.xspf", `<?xml version="1.0"?><playlist><trackList><track><location>c.mp4</location></track></trackList></playlist>`)
	got, err = Expand(xspf, false, 10)
	if err != nil || len(got) != 1 || got[0] != filepath.Join(dir, "c.mp4") {
		t.Fatalf("xspf %v %v", got, err)
	}
}

func TestFolderAndHLS(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.mp4", "")
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, sub, "b.mkv", "")
	write(t, dir, "notes.txt", "")
	got, err := Expand(dir, true, 10)
	if err != nil || len(got) != 2 || got[1] != filepath.Join(sub, "b.mkv") {
		t.Fatalf("folder %v %v", got, err)
	}
	got, _ = Expand(dir, false, 10)
	if len(got) != 1 {
		t.Fatalf("non-recursive %v", got)
	}
	hls := write(t, dir, "hls.m3u8", "#EXTM3U\n#EXT-X-TARGETDURATION:10\nseg.ts\n#EXT-X-ENDLIST\n")
	got, err = Expand(hls, false, 10)
	if err != nil || len(got) != 1 || got[0] != hls {
		t.Fatalf("hls %v %v", got, err)
	}
}

func TestInvalid(t *testing.T) {
	if _, err := Expand("/nonexistent", false, 10); err == nil {
		t.Fatal("want error")
	}
	dir := t.TempDir()
	pls := write(t, dir, "bad.pls", "[playlist]\n")
	if _, err := Expand(pls, false, 10); err == nil {
		t.Fatal("want pls error")
	}
}
