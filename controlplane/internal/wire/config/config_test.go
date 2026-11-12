package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveConfigPathAtomicAndNoTempLeft(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	if err := SaveConfigPath("/a.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := SaveConfigPath("/b.yaml"); err != nil {
		t.Fatal(err)
	}

	got, err := LoadConfigPath()
	if err != nil || got != "/b.yaml" {
		t.Fatalf("got %q, %v", got, err)
	}

	entries, _ := os.ReadDir(filepath.Join(home, subdir))
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
}
