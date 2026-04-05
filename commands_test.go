package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripSurroundingQuotes(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{`"hello"`, "hello"},
		{`'hello'`, "hello"},
		{`hello`, "hello"},
		{`""`, ""},
		{`''`, ""},
		{`"hello'`, `"hello'`},
		{`'hello"`, `'hello"`},
		{`a`, "a"},
		{``, ""},
		{`"it's quoted"`, "it's quoted"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stripSurroundingQuotes(tt.input)
			if got != tt.want {
				t.Errorf("stripSurroundingQuotes(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		input   string
		wantCmd string
		wantArg string
	}{
		{"readfile main.go", "readfile", "main.go"},
		{"findfile package main", "findfile", "package main"},
		{"respond hello world", "respond", "hello world"},
		{"readfile", "readfile", ""},
		{`readfile("main.go")`, "readfile", "main.go"},
		{`readfile('main.go')`, "readfile", "main.go"},
		{`findfile("fmt.Println")`, "findfile", "fmt.Println"},
		{"writefile path\ncontent", "writefile", "path\ncontent"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cmd, arg := splitCommand(tt.input)
			if cmd != tt.wantCmd || arg != tt.wantArg {
				t.Errorf("splitCommand(%q) = (%q, %q), want (%q, %q)", tt.input, cmd, arg, tt.wantCmd, tt.wantArg)
			}
		})
	}
}

func TestUnescapeLiteral(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{`hello\nworld`, "hello\nworld"},
		{`col1\tcol2`, "col1\tcol2"},
		{`no escapes`, "no escapes"},
		{`line1\nline2\nline3`, "line1\nline2\nline3"},
		{`\n\t\n`, "\n\t\n"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := unescapeLiteral(tt.input)
			if got != tt.want {
				t.Errorf("unescapeLiteral(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolvePath(t *testing.T) {
	pathMap := map[string]string{
		"main.go":                  "package main",
		"src/utils.go":             "package src",
		filepath.Clean("a/b/c.go"): "package abc",
	}

	tests := []struct {
		input  string
		wantOK bool
	}{
		{"main.go", true},
		{"src/utils.go", true},
		{"a/b/c.go", true},
		{"nonexistent.go", false},
		{"  main.go  ", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, ok := resolvePath(tt.input, pathMap)
			if ok != tt.wantOK {
				t.Errorf("resolvePath(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
		})
	}
}

func TestSplitHeaderContent(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantHeader  string
		wantContent string
		wantOK      bool
	}{
		{"real newline", "writefile path.go\nline1\nline2", "writefile path.go", "line1\nline2", true},
		{"literal newline", `writefile path.go\nline1\nline2`, "writefile path.go", "line1\nline2", true},
		{"no newline", "writefile path.go", "writefile path.go", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header, content, ok := splitHeaderContent(tt.input)
			if header != tt.wantHeader || content != tt.wantContent || ok != tt.wantOK {
				t.Errorf("splitHeaderContent(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.input, header, content, ok, tt.wantHeader, tt.wantContent, tt.wantOK)
			}
		})
	}
}

func TestExecReadFile(t *testing.T) {
	pathMap := map[string]string{
		"main.go": "package main\n\nfunc main() {}",
	}

	t.Run("existing file", func(t *testing.T) {
		result := execReadFile("main.go", pathMap)
		if result != pathMap["main.go"] {
			t.Errorf("got %q, want file contents", result)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		result := execReadFile("nope.go", pathMap)
		if !strings.Contains(result, "not found") {
			t.Errorf("expected 'not found' error, got %q", result)
		}
	})
}

func TestExecReadLines(t *testing.T) {
	pathMap := map[string]string{
		"test.txt": "line1\nline2\nline3\nline4\nline5",
	}

	tests := []struct {
		name    string
		arg     string
		wantErr string
		wantSub string
	}{
		{"valid range", "test.txt 2 4", "", "2: line2"},
		{"file not found", "nope.txt 1 2", "not found", ""},
		{"bad args", "test.txt abc def", "must be integers", ""},
		{"wrong arg count", "test.txt 1", "usage", ""},
		{"start > end", "test.txt 5 2", "startLine must be <= endLine", ""},
		{"clamps start", "test.txt 0 2", "", "1: line1"},
		{"clamps end", "test.txt 4 100", "", "5: line5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := execReadLines(tt.arg, pathMap)
			if tt.wantErr != "" && !strings.Contains(result, tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, result)
			}
			if tt.wantSub != "" && !strings.Contains(result, tt.wantSub) {
				t.Errorf("expected result containing %q, got %q", tt.wantSub, result)
			}
		})
	}
}

func TestExecFindFile(t *testing.T) {
	pathMap := map[string]string{
		"a.go": "package main\nfunc main() {}\n",
		"b.go": "package util\nfunc helper() {}\n",
	}

	t.Run("finds matches", func(t *testing.T) {
		result := execFindFile("package", pathMap)
		if !strings.Contains(result, "package") {
			t.Errorf("expected matches, got %q", result)
		}
	})

	t.Run("no matches", func(t *testing.T) {
		result := execFindFile("zzzznotfound", pathMap)
		if !strings.Contains(result, "no matches") {
			t.Errorf("expected 'no matches', got %q", result)
		}
	})

	t.Run("empty pattern", func(t *testing.T) {
		result := execFindFile("", pathMap)
		if !strings.Contains(result, "usage") {
			t.Errorf("expected usage error, got %q", result)
		}
	})

	t.Run("file-scoped findfile", func(t *testing.T) {
		result := execFindFile("package a.go", pathMap)
		if !strings.Contains(result, "a.go") {
			t.Errorf("expected match in a.go, got %q", result)
		}
		if strings.Contains(result, "b.go") {
			t.Errorf("should not match in b.go, got %q", result)
		}
	})

	t.Run("file-scoped findfile no match", func(t *testing.T) {
		result := execFindFile("helper a.go", pathMap)
		if !strings.Contains(result, "no matches") {
			t.Errorf("expected no matches in a.go, got %q", result)
		}
	})

	t.Run("caps at max results", func(t *testing.T) {
		bigMap := map[string]string{}
		var sb strings.Builder
		for i := range 100 {
			sb.Reset()
			for j := range 10 {
				sb.WriteString("match_line_")
				sb.WriteString(strings.Repeat("x", i+j))
				sb.WriteString("\n")
			}
			bigMap["file"+strings.Repeat("x", i)+".go"] = sb.String()
		}
		result := execFindFile("match_line_", bigMap)
		if !strings.Contains(result, "more matches omitted") {
			t.Errorf("expected truncation message, got %q", result)
		}
	})
}

func TestSplitFindFileArg(t *testing.T) {
	pathMap := map[string]string{
		"main.go":      "package main",
		"src/utils.go": "package src",
	}

	tests := []struct {
		name        string
		arg         string
		wantPattern string
		wantFile    string
	}{
		{"pattern only", "package main", "package main", ""},
		{"pattern with file", "logging main.go", "logging", "main.go"},
		{"pattern with nested file", "func src/utils.go", "func", "src/utils.go"},
		{"single word pattern", "main", "main", ""},
		{"nonexistent file suffix", "foo bar.txt", "foo bar.txt", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, file := splitFindFileArg(tt.arg, pathMap)
			if pattern != tt.wantPattern || file != tt.wantFile {
				t.Errorf("splitFindFileArg(%q) = (%q, %q), want (%q, %q)",
					tt.arg, pattern, file, tt.wantPattern, tt.wantFile)
			}
		})
	}
}

func TestExecWriteFile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	pathMap := map[string]string{}
	raw := "writefile test.txt\nhello world"
	result := execWriteFile(raw, pathMap)

	if !strings.Contains(result, "wrote") {
		t.Fatalf("expected success, got %q", result)
	}
	if pathMap["test.txt"] != "hello world" {
		t.Errorf("pathMap not updated, got %q", pathMap["test.txt"])
	}
	data, err := os.ReadFile("test.txt")
	if err != nil {
		t.Fatalf("file not created on disk: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("disk content = %q, want %q", string(data), "hello world")
	}
}

func TestExecWriteLines(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	pathMap := map[string]string{
		"test.txt": "line1\nline2\nline3\nline4\nline5",
	}
	os.WriteFile("test.txt", []byte(pathMap["test.txt"]), 0644)

	t.Run("replace middle lines", func(t *testing.T) {
		raw := "writelines test.txt 2 3\nNEW2\nNEW3"
		result := execWriteLines(raw, pathMap)
		if !strings.Contains(result, "wrote") {
			t.Fatalf("expected success, got %q", result)
		}
		lines := strings.Split(pathMap["test.txt"], "\n")
		if lines[1] != "NEW2" || lines[2] != "NEW3" {
			t.Errorf("unexpected content: %v", lines)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		raw := "writelines nope.txt 1 2\ncontent"
		result := execWriteLines(raw, pathMap)
		if !strings.Contains(result, "not found") {
			t.Errorf("expected not found error, got %q", result)
		}
	})

	t.Run("bad line numbers", func(t *testing.T) {
		raw := "writelines test.txt abc def\ncontent"
		result := execWriteLines(raw, pathMap)
		if !strings.Contains(result, "must be integers") {
			t.Errorf("expected integer error, got %q", result)
		}
	})

	t.Run("missing content", func(t *testing.T) {
		raw := "writelines test.txt 1 2"
		result := execWriteLines(raw, pathMap)
		if !strings.Contains(result, "usage") {
			t.Errorf("expected usage error, got %q", result)
		}
	})
}

func TestExecCreateFile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	pathMap := map[string]string{}
	result := execCreateFile("newfile.txt", pathMap)
	if !strings.Contains(result, "created") {
		t.Fatalf("expected 'created', got %q", result)
	}
	if _, ok := pathMap[filepath.Clean("newfile.txt")]; !ok {
		t.Error("pathMap not updated")
	}
	if _, err := os.Stat(filepath.Join(dir, "newfile.txt")); err != nil {
		t.Errorf("file not on disk: %v", err)
	}
}

func TestExecCommand_Dispatch(t *testing.T) {
	pathMap := map[string]string{
		"main.go": "package main",
	}

	t.Run("readfile dispatches", func(t *testing.T) {
		result := execCommand("readfile main.go", pathMap)
		if result != "package main" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("respond returns arg", func(t *testing.T) {
		result := execCommand("respond hello world", pathMap)
		if result != "hello world" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("unknown command", func(t *testing.T) {
		result := execCommand("foobar something", pathMap)
		if !strings.Contains(result, "unknown command") {
			t.Errorf("expected unknown command, got %q", result)
		}
	})
}

func TestCountRemainingMatches(t *testing.T) {
	pathMap := map[string]string{
		"a.go": "match\nmatch\nno",
		"b.go": "match\nno\nmatch",
	}
	remaining := []string{"match", "no", "match"}
	count := countRemainingMatches("match", "a.go", remaining, pathMap)
	// 2 from remaining lines of a.go, plus whatever comes after a.go in map iteration
	if count < 2 {
		t.Errorf("expected at least 2 remaining matches, got %d", count)
	}
}
