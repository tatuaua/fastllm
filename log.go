package main

import (
	"fmt"
	"os"
	"strings"
)

var logs []string

func logf(format string, args ...any) {
	logs = append(logs, fmt.Sprintf(format, args...))
}

func WriteLogs(path string) {
	os.WriteFile(path, []byte(strings.Join(logs, "\n")+"\n"), 0644)
}
