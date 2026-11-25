package engine

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// shotBackend writes a real PNG for a screenshot, and can fail after writing
// it the way a snapshot that errors late does.
type shotBackend struct {
	*fakeBackend
	failAfterWrite bool
	invalid        bool
}

func (b *shotBackend) Screenshot(path string) error {
	var data []byte
	if b.invalid {
		data = []byte("not a png")
	} else {
		img := image.NewRGBA(image.Rect(0, 0, 2, 2))
		img.Set(0, 0, color.RGBA{R: 200, A: 255})
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return err
		}
		data = buf.Bytes()
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	if b.failAfterWrite {
		return errors.New("snapshot failed after writing")
	}
	return nil
}

func newShotEngine(t *testing.T, backend *shotBackend) *Engine {
	t.Helper()
	backend.fakeBackend = newFake()
	eng := New(func(string, bool, string) (Backend, error) { return backend, nil })
	t.Cleanup(eng.Close)
	if err := eng.Load("movie.mkv"); err != nil {
		t.Fatal(err)
	}
	return eng
}

func TestScreenshotJPEGWritesAJPEGAndRemovesTheTemp(t *testing.T) {
	dir := t.TempDir()
	eng := newShotEngine(t, &shotBackend{})
	path := filepath.Join(dir, "frame.jpg")
	if err := eng.ScreenshotJPEG(path, 55); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 2 || raw[0] != 0xFF || raw[1] != 0xD8 {
		t.Fatalf("not a JPEG: %x", raw[:2])
	}
	if _, err := os.Stat(path + ".png"); err == nil {
		t.Fatal("the temporary PNG was left behind")
	}
}

func TestScreenshotJPEGCleansUpWhenTheSnapshotFails(t *testing.T) {
	dir := t.TempDir()
	eng := newShotEngine(t, &shotBackend{failAfterWrite: true})
	path := filepath.Join(dir, "frame.jpg")
	if err := eng.ScreenshotJPEG(path, 90); err == nil {
		t.Fatal("a failed snapshot reported success")
	}
	if _, err := os.Stat(path + ".png"); err == nil {
		t.Fatal("the temporary PNG survived a failed snapshot")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a JPEG was written from a failed snapshot")
	}
}

func TestScreenshotJPEGRejectsAnUndecodableFrame(t *testing.T) {
	dir := t.TempDir()
	eng := newShotEngine(t, &shotBackend{invalid: true})
	path := filepath.Join(dir, "frame.jpg")
	if err := eng.ScreenshotJPEG(path, 90); err == nil {
		t.Fatal("an undecodable frame reported success")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("a broken JPEG was left behind")
	}
	if _, err := os.Stat(path + ".png"); err == nil {
		t.Fatal("the temporary PNG survived")
	}
}
