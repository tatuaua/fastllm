package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

var logs []string

func logf(format string, args ...any) {
	logs = append(logs, fmt.Sprintf(format, args...))
}

func WriteLogs(path string) {
	os.WriteFile(path, []byte(strings.Join(logs, "\n")+"\n"), 0644)
}

const (
	CommandReadFile   = "readfile"
	CommandCreateFile = "createfile"
	CommandRespond    = "respond"
)

var commandToDescription = map[string]string{
	CommandReadFile:   "Used to read the contents of a file. Example usage: 'readfile <filename>'",
	CommandCreateFile: "Used to create a new file. Example usage: 'createfile <filename>'",
	CommandRespond:    "Used when the user's request has been fulfilled. Example usage: 'respond <response message>'",
}

type Prompt struct {
	UserPrompt string   `json:"user_prompt"`
	Paths      []string `json:"paths"`
}

type Response struct {
	Command string `json:"c"`
}

type ResponseMsg string

func (m *model) ExecCommand(command string) {
	sections := strings.Split(command, " ")
	baseCommand := sections[0]
	var arg string
	if len(sections) > 1 {
		arg = sections[1]
	}
	logf("[exec] Command=%q Target=%q", baseCommand, arg)
	switch baseCommand {
	case "readfile":
		logf("[exec] Reading file: %s", arg)
		file, err := os.ReadFile(arg)
		if err != nil {
			logf("[exec] Error reading file: %v", err)
			return
		}
		truncated := ""
		if len(file) > 50 {
			truncated = string(file)[:50]
		} else {
			truncated = string(file)
		}
		logf("Output: \n %s", truncated)
	case "createfile":
		_, err := os.Create(arg)
		if err != nil {
			logf("[exec] Error creating file: %v", err)
			return
		}
		m.pathMap[arg] = ""
	case "respond":
	default:
		logf("Unknown command: %s", command)
	}
}

func BuildPathMap() map[string]string {
	var pathMap = map[string]string{}
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
		ok, err := IsPlainText(path)
		if err != nil || !ok {
			logf("[scan] Skipped (binary): %s", rel)
			return nil
		}
		fileContent, err := os.ReadFile(rel)
		if err != nil {
			logf("[scan] Error reading file %s: %v", rel, err)
			return nil
		}
		logf("[scan] Added: %s (%d bytes)", rel, len(fileContent))
		pathMap[rel] = string(fileContent)
		return nil
	})

	return pathMap
}

func (m model) CallConverse(prompt Prompt) tea.Msg {
	promptAsJSON, err := json.Marshal(prompt)
	if err != nil {
		log.Fatalf("failed to marshal prompt to JSON: %v", err)
	}

	logf("Prompt JSON: %s", string(promptAsJSON))

	ctx := context.TODO()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}

	client := bedrockruntime.NewFromConfig(cfg)
	modelID := "eu.amazon.nova-micro-v1:0"

	var commandDescriptions string
	for cmd, desc := range commandToDescription {
		commandDescriptions += fmt.Sprintf("- %s: %s\n", cmd, desc)
	}

	input := &bedrockruntime.ConverseInput{
		ModelId: aws.String(modelID),
		System: []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{
				Value: "You are a senior system admin and your task is to provide custom commands based on the context and prompt given to you.\n" +
					"You must only output a JSON object in the following format: {\"c\":\"your command here\"}\n" +
					"Ensure commands are syntactically correct with proper spacing between commands and arguments.\n" +
					"Only output one command at a time.\n" +
					"Available commands:\n" + commandDescriptions +
					"Do not output anything other than the JSON object." +
					"Session history: \n" +
					m.history,
			},
		},
		Messages: []types.Message{
			{
				Role: types.ConversationRoleUser,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{
						Value: string(promptAsJSON),
					},
				},
			},
		},
	}

	logf("[llm] Calling Converse API (model: %s)", modelID)
	output, err := client.Converse(ctx, input)
	if err != nil {
		log.Fatalf("failed to call Converse API: %v", err)
	}
	logf("[llm] Received response")

	var response Response

	switch message := output.Output.(type) {
	case *types.ConverseOutputMemberMessage:
		for _, content := range message.Value.Content {
			switch textBlock := content.(type) {
			case *types.ContentBlockMemberText:
				logf("[llm] Raw response: %s", textBlock.Value)
				err := json.Unmarshal([]byte(textBlock.Value), &response)
				if err != nil {
					log.Fatalf("failed to unmarshal response JSON: %v", err)
				}
			}
		}
	default:
		logf("Unexpected output type received.")
	}
	return ResponseMsg(response.Command)
}

func IsPlainText(filePath string) (bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	buffer := make([]byte, 1024)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return false, err
	}

	for i := 0; i < n; i++ {
		b := buffer[i]
		if b == 0 {
			return false, nil
		}
		if b < 32 && b != 9 && b != 10 && b != 13 {
			return false, nil
		}
	}

	return true, nil
}
