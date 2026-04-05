package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	CommandReadFile   = "readfile"
	CommandCreateFile = "createfile"
	CommandReadLines  = "readlines"
	CommandWriteFile  = "writefile"
	CommandWriteLines = "writelines"
	CommandGrep       = "grep"
	CommandRespond    = "respond"

	maxGrepResults = 50
)

var availableCommands = map[string]string{
	CommandReadFile:   "Read entire file contents. Usage: 'readfile <path>'",
	CommandCreateFile: "Create a new empty file. Usage: 'createfile <path>'",
	CommandReadLines:  "Read specific lines from a file (1-indexed, inclusive). Usage: 'readlines <path> <startLine> <endLine>'",
	CommandWriteFile:  "Write entire file content (creates or overwrites). First line is path, rest is content. Usage: 'writefile <path>\\n<content>'",
	CommandWriteLines: "Replace lines in a file (1-indexed). Usage: 'writelines <path> <startLine> <endLine>\\n<content>'",
	CommandGrep:       "Search all files for a text pattern. Returns file:line:content matches (max 50). Usage: 'grep <pattern>'",
	CommandRespond:    "Return final answer to the user. Usage: 'respond <message>'",
}

// stripSurroundingQuotes removes a matching pair of surrounding single or
// double quotes from s, if present. Inner content is left untouched.
func stripSurroundingQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// splitCommand extracts the command name and argument from a raw command
// string. It handles both "cmd arg" and "cmd('arg')" / "cmd(\"arg\")" syntax
// that LLMs sometimes produce, and strips surrounding quotes from the argument.
func splitCommand(raw string) (cmd, arg string) {
	i := strings.IndexAny(raw, " (")
	if i < 0 {
		return raw, ""
	}
	cmd = raw[:i]
	arg = raw[i+1:]
	if raw[i] == '(' {
		arg = strings.TrimRight(arg, ")")
	}
	arg = stripSurroundingQuotes(arg)
	return cmd, arg
}

// unescapeLiteral replaces literal \n and \t sequences (two chars) with real
// newline/tab bytes. LLMs inside JSON often produce \\n which after JSON
// unmarshal becomes the two-char sequence \n rather than a newline byte.
func unescapeLiteral(s string) string {
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	return s
}

// resolvePath normalises a path from the LLM and looks it up in pathMap.
// It tries the raw path first, then filepath.Clean, then forward-slash
// variants. Returns the canonical key and true, or "" and false.
func resolvePath(raw string, pathMap map[string]string) (string, bool) {
	raw = strings.TrimSpace(raw)
	// Try raw.
	if _, ok := pathMap[raw]; ok {
		return raw, true
	}
	// Try cleaned.
	cleaned := filepath.Clean(raw)
	if _, ok := pathMap[cleaned]; ok {
		return cleaned, true
	}
	// Try with forward slashes converted to OS separator.
	slashed := filepath.Clean(strings.ReplaceAll(raw, "/", string(filepath.Separator)))
	if _, ok := pathMap[slashed]; ok {
		return slashed, true
	}
	return raw, false
}

// execCommand runs a single command and returns a textual result that can be
// fed back to the LLM as tool output. pathMap is updated in-place for
// commands that modify the workspace (e.g. createfile, writefile, writelines).
func execCommand(raw string, pathMap map[string]string) string {
	command, arg := splitCommand(raw)

	logf("[exec] Command=%q Target=%q", command, arg)

	switch command {
	case CommandReadFile:
		return execReadFile(arg, pathMap)
	case CommandCreateFile:
		return execCreateFile(arg, pathMap)
	case CommandReadLines:
		return execReadLines(arg, pathMap)
	case CommandWriteFile:
		return execWriteFile(raw, pathMap)
	case CommandWriteLines:
		return execWriteLines(raw, pathMap)
	case CommandGrep:
		return execGrep(arg, pathMap)
	case CommandRespond:
		return arg
	default:
		logf("Unknown command: %s", raw)
		return fmt.Sprintf("unknown command: %s", command)
	}
}

func execReadFile(arg string, pathMap map[string]string) string {
	key, ok := resolvePath(arg, pathMap)
	if !ok {
		logf("[exec] File not found in pathMap: %s", arg)
		return fmt.Sprintf("error: file %q not found", arg)
	}
	logf("[exec] Read %d bytes from %s", len(pathMap[key]), key)
	return pathMap[key]
}

func execCreateFile(arg string, pathMap map[string]string) string {
	arg = strings.TrimSpace(arg)
	clean := filepath.Clean(arg)
	f, err := os.Create(clean)
	if err != nil {
		logf("[exec] Error creating file: %v", err)
		return fmt.Sprintf("error creating file: %v", err)
	}
	f.Close()
	pathMap[clean] = ""
	return fmt.Sprintf("created file: %s", clean)
}

func execReadLines(arg string, pathMap map[string]string) string {
	parts := strings.Fields(arg)
	if len(parts) != 3 {
		return "error: usage: readlines <path> <startLine> <endLine>"
	}
	key, ok := resolvePath(parts[0], pathMap)
	if !ok {
		return fmt.Sprintf("error: file %q not found", parts[0])
	}
	start, err1 := strconv.Atoi(parts[1])
	end, err2 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil {
		return "error: startLine and endLine must be integers"
	}
	content := pathMap[key]
	lines := strings.Split(content, "\n")
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return "error: startLine must be <= endLine"
	}
	var b strings.Builder
	for i := start - 1; i < end; i++ {
		fmt.Fprintf(&b, "%d: %s\n", i+1, lines[i])
	}
	logf("[exec] ReadLines %s [%d-%d] (%d lines)", key, start, end, end-start+1)
	return b.String()
}

// splitHeaderContent splits a raw command string into header (first line) and
// content (remaining lines). It handles both real newlines and literal \n
// sequences that LLMs produce inside JSON strings.
func splitHeaderContent(raw string) (header, content string, ok bool) {
	// Try real newline first.
	if h, c, found := strings.Cut(raw, "\n"); found {
		return h, c, true
	}
	// Fall back to literal \n sequence.
	if h, c, found := strings.Cut(raw, `\n`); found {
		return h, unescapeLiteral(c), true
	}
	return raw, "", false
}

func execWriteFile(raw string, pathMap map[string]string) string {
	// Format: writefile <path>\n<content>
	_, afterCmd, _ := strings.Cut(raw, " ")
	header, content, ok := splitHeaderContent(afterCmd)
	if !ok {
		return "error: usage: writefile <path>\\n<content>"
	}
	file := strings.TrimSpace(header)
	file = filepath.Clean(file)

	pathMap[file] = content
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		logf("[exec] Error writing file %s: %v", file, err)
		return fmt.Sprintf("error writing file: %v", err)
	}
	logf("[exec] WriteFile %s (%d bytes)", file, len(content))
	return fmt.Sprintf("wrote %d bytes to %s", len(content), file)
}

func execWriteLines(raw string, pathMap map[string]string) string {
	// Format: writelines <path> <start> <end>\n<content>
	header, content, hasContent := splitHeaderContent(raw)
	if !hasContent {
		return "error: usage: writelines <path> <startLine> <endLine>\\n<content>"
	}
	// Strip command name from header.
	_, headerArgs, _ := strings.Cut(header, " ")
	parts := strings.Fields(headerArgs)
	if len(parts) != 3 {
		return "error: usage: writelines <path> <startLine> <endLine>\\n<content>"
	}
	key, ok := resolvePath(parts[0], pathMap)
	if !ok {
		return fmt.Sprintf("error: file %q not found", parts[0])
	}
	start, err1 := strconv.Atoi(parts[1])
	end, err2 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil {
		return "error: startLine and endLine must be integers"
	}
	existing := pathMap[key]

	lines := strings.Split(existing, "\n")
	if start < 1 || start > len(lines)+1 {
		return fmt.Sprintf("error: startLine %d out of range (file has %d lines)", start, len(lines))
	}
	if end > len(lines) {
		end = len(lines)
	}
	if end < start-1 {
		end = start - 1
	}

	newLines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines)-(end-start+1)+len(newLines))
	result = append(result, lines[:start-1]...)
	result = append(result, newLines...)
	if end < len(lines) {
		result = append(result, lines[end:]...)
	}

	updated := strings.Join(result, "\n")
	pathMap[key] = updated
	if err := os.WriteFile(key, []byte(updated), 0644); err != nil {
		logf("[exec] Error writing file %s: %v", key, err)
		return fmt.Sprintf("error writing file: %v", err)
	}
	logf("[exec] WriteLines %s [%d-%d] replaced with %d new lines", key, start, end, len(newLines))
	return fmt.Sprintf("wrote %d lines to %s (replaced lines %d-%d)", len(newLines), key, start, end)
}

func execGrep(pattern string, pathMap map[string]string) string {
	if pattern == "" {
		return "error: usage: grep <pattern>"
	}
	var b strings.Builder
	count := 0
	for file, content := range pathMap {
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.Contains(line, pattern) {
				fmt.Fprintf(&b, "%s:%d: %s\n", file, i+1, line)
				count++
				if count >= maxGrepResults {
					total := countRemainingMatches(pattern, file, lines[i+1:], pathMap)
					fmt.Fprintf(&b, "... %d more matches omitted\n", total)
					return b.String()
				}
			}
		}
	}
	if count == 0 {
		return fmt.Sprintf("no matches found for %q", pattern)
	}
	logf("[exec] Grep %q: %d matches", pattern, count)
	return b.String()
}

// countRemainingMatches counts how many more matches exist beyond the cap.
func countRemainingMatches(pattern, currentFile string, remainingLines []string, pathMap map[string]string) int {
	count := 0
	for _, line := range remainingLines {
		if strings.Contains(line, pattern) {
			count++
		}
	}
	started := false
	for file, content := range pathMap {
		if file == currentFile {
			started = true
			continue
		}
		if !started {
			continue
		}
		for _, line := range strings.Split(content, "\n") {
			if strings.Contains(line, pattern) {
				count++
			}
		}
	}
	return count
}
