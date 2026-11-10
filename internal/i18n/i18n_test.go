package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTranslatesContextAndFallsBack(t *testing.T) {
	dir := t.TempDir()
	body := `<?xml version="1.0" encoding="utf-8"?>
<TS version="2.1" language="pl">
  <context>
    <name>Main Window</name>
    <message><source>Settings</source><translation>Ustawienia</translation></message>
    <message><source>Recent</source><translation type="unfinished"/></message>
  </context>
</TS>`
	if err := os.WriteFile(filepath.Join(dir, "pl_PL.ts"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	translator, err := Load(dir, "pl_PL")
	if err != nil {
		t.Fatal(err)
	}
	if got := translator.Tr("Main Window", "Settings"); got != "Ustawienia" {
		t.Fatalf("translation %q", got)
	}
	if got := translator.Tr("Main Window", "Recent"); got != "Recent" {
		t.Fatalf("unfinished fallback %q", got)
	}
	if got := translator.Tr("Missing", "Settings"); got != "Ustawienia" {
		t.Fatalf("source fallback %q", got)
	}
}

func TestEnglishNeedsNoFile(t *testing.T) {
	translator, err := Load(t.TempDir(), "en_US")
	if err != nil {
		t.Fatal(err)
	}
	if got := translator.Tr("Main Window", "Settings"); got != "Settings" {
		t.Fatalf("English %q", got)
	}
}

func TestLanguageNames(t *testing.T) {
	if Name("pl_PL") != "Polski" || Name("unknown") != "English" {
		t.Fatal("language names")
	}
}
