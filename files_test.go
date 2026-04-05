package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsPlainText(t *testing.T) {
	dir := t.TempDir()

	t.Run("text file", func(t *testing.T) {
		path := filepath.Join(dir, "hello.txt")
		os.WriteFile(path, []byte("hello world\nline two\n"), 0644)
		ok, err := isPlainText(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Error("expected true for text file")
		}
	})

	t.Run("binary file with null byte", func(t *testing.T) {
		path := filepath.Join(dir, "binary.bin")
		os.WriteFile(path, []byte{0x48, 0x65, 0x6c, 0x00, 0x6f}, 0644)
		ok, err := isPlainText(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Error("expected false for binary file")
		}
	})

	t.Run("file with control chars", func(t *testing.T) {
		path := filepath.Join(dir, "control.dat")
		os.WriteFile(path, []byte{0x01, 0x02, 0x03}, 0644)
		ok, err := isPlainText(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Error("expected false for file with control chars")
		}
	})

	t.Run("file with tabs and newlines", func(t *testing.T) {
		path := filepath.Join(dir, "tabbed.txt")
		os.WriteFile(path, []byte("col1\tcol2\nrow1\trow2\r\n"), 0644)
		ok, err := isPlainText(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Error("expected true for file with tabs/newlines")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		path := filepath.Join(dir, "empty.txt")
		os.WriteFile(path, []byte{}, 0644)
		ok, err := isPlainText(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Error("expected true for empty file")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := isPlainText(filepath.Join(dir, "nope.txt"))
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func TestBuildPathMap(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	// Create some test files.
	os.WriteFile("hello.go", []byte("package main"), 0644)
	os.MkdirAll("sub", 0755)
	os.WriteFile(filepath.Join("sub", "util.go"), []byte("package sub"), 0644)

	// Create a .git dir that should be skipped.
	os.MkdirAll(".git", 0755)
	os.WriteFile(filepath.Join(".git", "config"), []byte("gitconfig"), 0644)

	// Create a binary file that should be skipped.
	os.WriteFile("binary.exe", []byte{0x00, 0x01, 0x02}, 0644)

	pm := BuildPathMap()

	if _, ok := pm["hello.go"]; !ok {
		t.Error("missing hello.go")
	}
	if _, ok := pm[filepath.Join("sub", "util.go")]; !ok {
		t.Error("missing sub/util.go")
	}
	if _, ok := pm[filepath.Join(".git", "config")]; ok {
		t.Error(".git should be skipped")
	}
	if _, ok := pm["binary.exe"]; ok {
		t.Error("binary files should be skipped")
	}
}
