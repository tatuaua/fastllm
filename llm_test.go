package main

import (
	"encoding/json"
	"testing"
)

func TestSanitizeJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"plain json",
			`{"c":"readfile main.go"}`,
			`{"c":"readfile main.go"}`,
		},
		{
			"markdown fenced",
			"```json\n{\"c\":\"readfile main.go\"}\n```",
			`{"c":"readfile main.go"}`,
		},
		{
			"markdown fenced with spaces",
			"  ```json\n{\"c\":\"respond hello\"}\n```  ",
			`{"c":"respond hello"}`,
		},
		{
			"backslash continuation",
			"{\"c\":\"writefile test.go\\\npackage main\"}",
			"{\"c\":\"writefile test.go\npackage main\"}",
		},
		{
			"invalid single quote escape",
			`{"c":"respond it\'s done"}`,
			`{"c":"respond it's done"}`,
		},
		{
			"combined fixes",
			"```\n{\"c\":\"respond it\\'s done\"}\n```",
			`{"c":"respond it's done"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeJSON(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLLMResponse_ParseCommands(t *testing.T) {
	t.Run("single string", func(t *testing.T) {
		resp := llmResponse{Commands: json.RawMessage(`"readfile main.go"`)}
		cmds := resp.parseCommands()
		if len(cmds) != 1 || cmds[0] != "readfile main.go" {
			t.Errorf("got %v", cmds)
		}
	})

	t.Run("array of strings", func(t *testing.T) {
		resp := llmResponse{Commands: json.RawMessage(`["readfile a.go","readfile b.go"]`)}
		cmds := resp.parseCommands()
		if len(cmds) != 2 {
			t.Fatalf("expected 2 commands, got %d", len(cmds))
		}
		if cmds[0] != "readfile a.go" || cmds[1] != "readfile b.go" {
			t.Errorf("got %v", cmds)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		resp := llmResponse{Commands: json.RawMessage(`{bad}`)}
		cmds := resp.parseCommands()
		if cmds != nil {
			t.Errorf("expected nil, got %v", cmds)
		}
	})

	t.Run("empty array", func(t *testing.T) {
		resp := llmResponse{Commands: json.RawMessage(`[]`)}
		cmds := resp.parseCommands()
		if len(cmds) != 0 {
			t.Errorf("expected empty, got %v", cmds)
		}
	})
}

func TestBuildSystemPrompt(t *testing.T) {
	prompt := buildSystemPrompt("user: hi\nagent: hello\n", "help me code")

	// Should contain the original request.
	if got := prompt; !contains(got, "help me code") {
		t.Error("missing original request")
	}
	// Should contain available commands.
	if !contains(prompt, "readfile") {
		t.Error("missing readfile command")
	}
	if !contains(prompt, "findfile") {
		t.Error("missing findfile command")
	}
	// Should contain history.
	if !contains(prompt, "user: hi") {
		t.Error("missing session history")
	}
	// Should contain readfile guidance.
	if !contains(prompt, "under 50 lines") {
		t.Error("missing readfile size guidance")
	}
}

func TestBuildPathLengths(t *testing.T) {
	pathMap := map[string]string{
		"empty.go":   "",
		"one.go":     "single line",
		"three.go":   "a\nb\nc",
		"newline.go": "a\n",
	}

	got := buildPathLengths(pathMap)

	tests := []struct {
		path string
		want int
	}{
		{"empty.go", 0},
		{"one.go", 1},
		{"three.go", 3},
		{"newline.go", 2},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got[tt.path] != tt.want {
				t.Errorf("buildPathLengths[%q] = %d, want %d", tt.path, got[tt.path], tt.want)
			}
		})
	}

	if len(got) != len(pathMap) {
		t.Errorf("expected %d entries, got %d", len(pathMap), len(got))
	}
}

func contains(s, sub string) bool {
	return len(s) > 0 && len(sub) > 0 && len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
