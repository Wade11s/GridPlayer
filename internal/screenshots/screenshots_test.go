package screenshots

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 9, 24, 13, 5, 9, 0, time.UTC)

func TestRenderFillsTheFrozenSpecifiers(t *testing.T) {
	for _, tt := range []struct {
		template string
		title    string
		posMS    int64
		number   int
		want     string
	}{
		{"%F_%P", "plane-a.mp4", 3723456, 1, "plane-a_01-02-03.456"},
		{"%f", "feature.mkv", 0, 1, "feature.mkv"},
		{"%p", "x.mp4", 61000, 1, "00-01-01"},
		{"%n", "x.mp4", 0, 7, "0007"},
		{"%D", "x.mp4", 0, 1, "2026-09-24"},
		{"%T", "x.mp4", 0, 1, "2026-09-24_13-05-09"},
		{"%tY%tm%td", "x.mp4", 0, 1, "20260924"},
		{"%H% M", "x.mp4", 0, 1, "%H% M"},
		{"100%%", "x.mp4", 0, 1, "100%"},
		{"%q", "x.mp4", 0, 1, "%q"},
		{"end%", "x.mp4", 0, 1, "end%"},
	} {
		if got := Render(tt.template, tt.title, tt.posMS, fixedNow, tt.number); got != tt.want {
			t.Errorf("Render(%q) = %q, want %q", tt.template, got, tt.want)
		}
	}
}

func TestRenderSanitizesLikeTheFrozenNames(t *testing.T) {
	for _, tt := range []struct {
		title string
		want  string
	}{
		{"https://example.com/watch?v=1", "https___example.com_watch_v=1"},
		{"a  b\tc", "a b c"},
		{"trailing. ", "trailing"},
		{"", "screenshot"},
		{"con", "_con"},
		{"CON.txt", "_CON.txt"},
		{"part.2 of 3", "part.2 of 3"},
		// The frozen s+ takes the C0 separators, to one space.
		{"ab", "a b"},
		// The extension limit counts characters, not bytes.
		{"x.题题", "x"},
	} {
		if got := Render("%f", tt.title, 0, fixedNow, 1); got != tt.want {
			t.Errorf("Render(%%f, %q) = %q, want %q", tt.title, got, tt.want)
		}
	}
}

func TestUnknownAndCounter(t *testing.T) {
	if got := Unknown("%F_%P_%q_%tZ_end%"); !reflect.DeepEqual(got, []string{"%q", "%tZ", "%"}) {
		t.Fatalf("unknown = %v", got)
	}
	if !HasCounter("%F_%n") || HasCounter("%F_%P") {
		t.Fatal("counter detection is wrong")
	}
}

func TestFormatNamesAndExtensions(t *testing.T) {
	if FormatFromName("png") != PNG || FormatFromName("PNG") != PNG || FormatFromName("jpg") != JPG {
		t.Fatal("format parsing is wrong")
	}
	if PNG.Name() != "png" || PNG.Extension() != "png" || JPG.Extension() != "jpg" {
		t.Fatal("format names are wrong")
	}
}

func TestDirFallsBackToTheDataFolder(t *testing.T) {
	if got := Dir("", "/data"); got != filepath.Join("/data", DirName) {
		t.Fatalf("default dir = %q", got)
	}
	if got := Dir("  /tmp/shots  ", "/data"); got != "/tmp/shots" {
		t.Fatalf("configured dir = %q", got)
	}
}

func TestNextPathCountsAndSuffixes(t *testing.T) {
	dir := t.TempDir()
	first, err := NextPath(dir, "%F_%n", "a.mp4", 0, fixedNow, "png")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(first) != "a_0001.png" {
		t.Fatalf("first = %q", filepath.Base(first))
	}
	if err := os.WriteFile(first, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := NextPath(dir, "%F_%n", "a.mp4", 0, fixedNow, "png")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(second) != "a_0002.png" {
		t.Fatalf("second = %q", filepath.Base(second))
	}

	plain, err := NextPath(dir, "%F", "b.mp4", 0, fixedNow, "jpg")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(plain) != "b.jpg" {
		t.Fatalf("plain = %q", filepath.Base(plain))
	}
	if err := os.WriteFile(plain, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	suffixed, err := NextPath(dir, "%F", "b.mp4", 0, fixedNow, "jpg")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(suffixed) != "b (2).jpg" {
		t.Fatalf("suffixed = %q", filepath.Base(suffixed))
	}
}

func TestPositionFormatsHours(t *testing.T) {
	if got := Position(3723456, true); got != "01-02-03.456" {
		t.Fatalf("position = %q", got)
	}
	if got := Position(-5, false); got != "00-00-00" {
		t.Fatalf("negative position = %q", got)
	}
}
