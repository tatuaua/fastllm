package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	CommandReadFile   = "readfile"
	CommandCreateFile = "createfile"
	CommandRespond    = "respond"
)

var availableCommands = map[string]string{
	CommandReadFile:   "Used to read the contents of a file. Example usage: 'readfile <filename>'",
	CommandCreateFile: "Used to create a new file. Example usage: 'createfile <filename>'",
	CommandRespond:    "Used when the user's request has been fulfilled. Example usage: 'respond <response message>'",
}

// execCommand runs a single command and returns a textual result that can be
// fed back to the LLM as tool output. pathMap is updated in-place for
// commands that modify the workspace (e.g. createfile).
func execCommand(raw string, pathMap map[string]string) string {
	command, arg, _ := strings.Cut(raw, " ")

	logf("[exec] Command=%q Target=%q", command, arg)

	switch command {
	case CommandReadFile:
		logf("[exec] Reading file: %s", arg)
		content, err := os.ReadFile(arg)
		if err != nil {
			logf("[exec] Error reading file: %v", err)
			return fmt.Sprintf("error reading file: %v", err)
		}
		logf("[exec] Read %d bytes from %s", len(content), arg)
		return string(content)

	case CommandCreateFile:
		_, err := os.Create(arg)
		if err != nil {
			logf("[exec] Error creating file: %v", err)
			return fmt.Sprintf("error creating file: %v", err)
		}
		pathMap[arg] = ""
		return fmt.Sprintf("created file: %s", arg)

	case CommandRespond:
		return arg

	default:
		logf("Unknown command: %s", raw)
		return fmt.Sprintf("unknown command: %s", command)
	}
}
