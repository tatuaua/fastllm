package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

const (
	modelID      = "eu.amazon.nova-micro-v1:0"
	maxLoopSteps = 20
)

// Prompt is the structured input sent to the LLM.
type Prompt struct {
	UserPrompt string   `json:"user_prompt"`
	Paths      []string `json:"existing_paths"`
}

// LLMResponse is the JSON structure returned by the LLM.
type LLMResponse struct {
	Command string `json:"c"`
}

// ResponseMsg carries the LLM's command string back through Bubble Tea.
type ResponseMsg string

// converseCmd returns a tea.Cmd that runs the agent loop in the background.
// The LLM is called repeatedly — executing tool commands and feeding results
// back — until it issues a "respond" command with a final answer.
func (m model) converseCmd() tea.Cmd {
	paths := make([]string, 0, len(m.pathMap))
	for p := range m.pathMap {
		paths = append(paths, p)
	}

	prompt := Prompt{
		UserPrompt: m.textInput.Value(),
		Paths:      paths,
	}

	history := m.history
	pathMap := m.pathMap // shared map reference

	return func() tea.Msg {
		return runAgentLoop(prompt, history, pathMap)
	}
}

// runAgentLoop calls the LLM in a loop, executing tool commands and feeding
// results back, until the LLM sends a "respond" command or the step limit is
// reached.
func runAgentLoop(prompt Prompt, history string, pathMap map[string]string) tea.Msg {
	for step := range maxLoopSteps {
		resp := callLLM(prompt, history)
		raw := string(resp)
		command, _, _ := strings.Cut(raw, " ")

		logf("[loop] Step %d: command=%q", step+1, command)

		if command == CommandRespond {
			_, arg, _ := strings.Cut(raw, " ")
			return ResponseMsg(arg)
		}

		result := execCommand(raw, pathMap)
		history += fmt.Sprintf("tool_call: %s\ntool_result: %s\n", raw, result)

		// Refresh paths in case createfile added a new entry.
		prompt.Paths = make([]string, 0, len(pathMap))
		for p := range pathMap {
			prompt.Paths = append(prompt.Paths, p)
		}
	}

	logf("[loop] Reached max steps (%d), forcing stop", maxLoopSteps)
	return ResponseMsg("I ran out of steps before completing the task.")
}

// callLLM sends a single prompt to AWS Bedrock and returns the parsed command.
func callLLM(prompt Prompt, history string) ResponseMsg {
	promptJSON, err := json.Marshal(prompt)
	if err != nil {
		log.Fatalf("failed to marshal prompt to JSON: %v", err)
	}
	logf("Prompt JSON: %s", string(promptJSON))

	ctx := context.TODO()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("unable to load SDK config: %v", err)
	}

	client := bedrockruntime.NewFromConfig(cfg)

	systemPrompt := buildSystemPrompt(history)

	input := &bedrockruntime.ConverseInput{
		ModelId: aws.String(modelID),
		System: []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{Value: systemPrompt},
		},
		Messages: []types.Message{
			{
				Role: types.ConversationRoleUser,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{Value: string(promptJSON)},
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

	return parseConverseOutput(output)
}

// buildSystemPrompt constructs the system instruction for the LLM.
func buildSystemPrompt(history string) string {
	var commandList string
	for cmd, desc := range availableCommands {
		commandList += fmt.Sprintf("- %s: %s\n", cmd, desc)
	}

	return "You are a senior system admin and your task is to provide custom commands " +
		"based on the context and prompt given to you.\n" +
		"The 'existing_paths' field contains existing paths to files and directories.\n" +
		"You must only output a JSON object in the following format: {\"c\":\"your command here\"}\n" +
		"Ensure commands are syntactically correct with proper spacing between commands and arguments.\n" +
		"Only output one command at a time.\n" +
		"Available commands:\n" + commandList +
		"Do not output anything other than the JSON object." +
		"Session history:\n" + history
}

// parseConverseOutput extracts the command from the Bedrock Converse response.
func parseConverseOutput(output *bedrockruntime.ConverseOutput) ResponseMsg {
	var resp LLMResponse

	msg, ok := output.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		logf("Unexpected output type received.")
		return ""
	}

	for _, content := range msg.Value.Content {
		if textBlock, ok := content.(*types.ContentBlockMemberText); ok {
			logf("[llm] Raw response: %s", textBlock.Value)
			if err := json.Unmarshal([]byte(textBlock.Value), &resp); err != nil {
				log.Fatalf("failed to unmarshal response JSON: %v", err)
			}
		}
	}

	return ResponseMsg(resp.Command)
}
