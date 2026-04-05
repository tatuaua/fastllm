package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// BuildPathMap walks the current directory and returns a map of relative file
// paths to their contents, skipping binary files and common non-source dirs.
func BuildPathMap() map[string]string {
	pathMap := map[string]string{}

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get current directory: %v", err)
	}
	logf("[scan] Walking directory: %s", cwd)

	filepath.WalkDir(cwd, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, _ := filepath.Rel(cwd, path)

		ok, err := isPlainText(path)
		if err != nil || !ok {
			logf("[scan] Skipped (binary): %s", rel)
			return nil
		}

		content, err := os.ReadFile(rel)
		if err != nil {
			logf("[scan] Error reading file %s: %v", rel, err)
			return nil
		}

		logf("[scan] Added: %s (%d bytes)", rel, len(content))
		pathMap[rel] = string(content)
		return nil
	})

	return pathMap
}

// isPlainText checks whether a file appears to be a text file by inspecting
// its first 1024 bytes for null or non-printable control characters.
func isPlainText(filePath string) (bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	buf := make([]byte, 1024)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return false, err
	}

	for i := 0; i < n; i++ {
		b := buf[i]
		if b == 0 {
			return false, nil
		}
		if b < 32 && b != '\t' && b != '\n' && b != '\r' {
			return false, nil
		}
	}

	return true, nil
}
