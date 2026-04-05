package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	smithyauth "github.com/aws/smithy-go/auth"
	"github.com/aws/smithy-go/auth/bearer"
)

const (
	modelID      = "eu.amazon.nova-pro-v1:0"
	maxLoopSteps = 20
)

// ToolExec records a single tool call and its result for multi-command prompts.
type ToolExec struct {
	Call   string `json:"call"`
	Result string `json:"result"`
}

// Prompt is the structured input sent to the LLM.
type Prompt struct {
	UserPrompt string         `json:"user_prompt,omitempty"`
	ToolCalls  []ToolExec     `json:"tool_calls,omitempty"`
	Paths      map[string]int `json:"existing_paths"`
}

// llmResponse is the raw JSON structure returned by the LLM.
// The "c" field can be either a single string or an array of strings.
type llmResponse struct {
	Commands json.RawMessage `json:"c"`
}

// parseCommands extracts one or more command strings from the raw JSON "c" field.
func (r *llmResponse) parseCommands() []string {
	// Try array first (fast path for batch).
	var arr []string
	if err := json.Unmarshal(r.Commands, &arr); err == nil {
		return arr
	}
	// Fall back to single string.
	var single string
	if err := json.Unmarshal(r.Commands, &single); err == nil {
		return []string{single}
	}
	logf("[llm] Failed to parse commands field: %s", string(r.Commands))
	return nil
}

// Message types sent via the update channel.

// ResponseMsg carries the LLM's final answer.
type ResponseMsg string

// ToolCallMsg reports a single tool execution to the UI.
type ToolCallMsg struct {
	Command string
	Result  string
}

// AgentDoneMsg signals the agent loop has finished.
type AgentDoneMsg struct{}

// waitForUpdate returns a tea.Cmd that blocks until the next message arrives on ch.
func waitForUpdate(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

// converseCmd returns a tea.Cmd that starts the agent loop and yields the
// first intermediate message. Subsequent messages are pulled via waitForUpdate.
func (m *model) converseCmd() tea.Cmd {
	prompt := Prompt{
		UserPrompt: m.textInput.Value(),
		Paths:      buildPathLengths(m.pathMap),
	}

	history := m.rawHistory
	pathMap := m.pathMap

	ch := make(chan tea.Msg, 64)
	m.updates = ch

	go runAgentLoop(prompt, history, pathMap, ch)

	return waitForUpdate(ch)
}

// runAgentLoop calls the LLM in a loop, executing tool commands and sending
// intermediate ToolCallMsg updates via ch. When done, sends ResponseMsg + AgentDoneMsg.
func runAgentLoop(prompt Prompt, history string, pathMap map[string]string, ch chan<- tea.Msg) {
	defer func() { ch <- AgentDoneMsg{} }()

	originalRequest := prompt.UserPrompt

	for step := range maxLoopSteps {
		commands := callLLM(prompt, history, originalRequest)

		logf("[loop] Step %d: %d command(s)", step+1, len(commands))

		var toolExecs []ToolExec
		seen := make(map[string]bool)

		for _, raw := range commands {
			cmd, arg := splitCommand(raw)

			if cmd == CommandRespond {
				ch <- ResponseMsg(arg)
				return
			}

			// Skip duplicate commands in the same batch.
			if seen[raw] {
				logf("[loop] Skipping duplicate command: %s", raw)
				continue
			}
			seen[raw] = true

			result := execCommand(raw, pathMap)
			ch <- ToolCallMsg{Command: raw, Result: result}
			toolExecs = append(toolExecs, ToolExec{Call: raw, Result: result})
		}

		prompt = Prompt{
			ToolCalls: toolExecs,
			Paths:     buildPathLengths(pathMap),
		}
	}

	logf("[loop] Reached max steps (%d), forcing stop", maxLoopSteps)
	ch <- ResponseMsg("I ran out of steps before completing the task.")
}

// callLLM sends a single prompt to AWS Bedrock and returns the parsed command(s).
func callLLM(prompt Prompt, history string, originalRequest string) []string {
	promptJSON, err := json.Marshal(prompt)
	if err != nil {
		log.Fatalf("failed to marshal prompt to JSON: %v", err)
	}
	logf("Prompt JSON: %s", string(promptJSON))

	ctx := context.TODO()

	apiKey := os.Getenv("BEDROCK_API_KEY")
	if apiKey == "" {
		log.Fatal("BEDROCK_API_KEY environment variable is not set")
	}

	client := bedrockruntime.New(bedrockruntime.Options{
		Region: os.Getenv("AWS_REGION"),
		BearerAuthTokenProvider: bearer.StaticTokenProvider{
			Token: bearer.Token{Value: apiKey},
		},
		AuthSchemeResolver: &bearerAuthResolver{},
	})

	systemPrompt := buildSystemPrompt(history, originalRequest)

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

// bearerAuthResolver forces the SDK to use bearer token auth (Bedrock API keys).
type bearerAuthResolver struct{}

func (*bearerAuthResolver) ResolveAuthSchemes(_ context.Context, _ *bedrockruntime.AuthResolverParameters) ([]*smithyauth.Option, error) {
	return []*smithyauth.Option{
		{SchemeID: "smithy.api#httpBearerAuth"},
	}, nil
}

// buildPathLengths returns a map of file paths to their line counts.
func buildPathLengths(pathMap map[string]string) map[string]int {
	m := make(map[string]int, len(pathMap))
	for path, content := range pathMap {
		if content == "" {
			m[path] = 0
		} else {
			m[path] = strings.Count(content, "\n") + 1
		}
	}
	return m
}

// buildSystemPrompt constructs the system instruction for the LLM.
func buildSystemPrompt(history string, originalRequest string) string {
	var commandList strings.Builder
	for cmd, desc := range availableCommands {
		fmt.Fprintf(&commandList, "- %s: %s\n", cmd, desc)
	}

	return "You are a senior software engineer and coding agent. Your task is to fulfill the user's request " +
		"by using the available tool commands.\n" +
		"The user's original request: " + originalRequest + "\n" +
		"Use tool commands to gather information and make changes, then use 'respond' with your final answer.\n" +
		"The 'existing_paths' field maps each file path to its line count.\n" +
		"IMPORTANT RULES:\n" +
		"- Use forward slashes in paths (e.g. temp/main.go, not temp\\\\main.go).\n" +
		"- Do NOT wrap command arguments in quotes. Write: findfile package main, NOT: findfile 'package main'.\n" +
		"- For questions, explanations, or conversational messages, use 'respond' immediately. Do NOT create files for explanations.\n" +
		"- Do NOT repeat commands that already succeeded in previous steps.\n" +
		"- Do NOT use createfile. Use writefile to create OR overwrite files in one step.\n" +
		"- PREFER findfile and readlines over readfile. Only use readfile on small files (under 50 lines). " +
		"For larger files, use findfile to find relevant lines, then readlines to read specific sections. " +
		"You can scope findfile to a single file: 'findfile <pattern> <path>'.\n" +
		"You must output a JSON object with a \"c\" field containing commands.\n" +
		"You may batch multiple independent commands in one response for speed:\n" +
		"  Single: {\"c\":\"readfile main.go\"}\n" +
		"  Batch:  {\"c\":[\"readlines main.go 1 20\",\"readlines go.mod 1 10\"]}\n" +
		"When batching, commands run in order. Do NOT batch commands that depend on each other's results.\n" +
		"For writefile, put the path then a newline then the content. Example:\n" +
		"  {\"c\":\"writefile temp/main.go\\npackage main\\n\\nimport \\\"fmt\\\"\\n\\nfunc main() {\\n\\tfmt.Println(\\\"hello\\\")\\n}\"}\n" +
		"Use writelines only for surgical line-range edits.\n" +
		"Available commands:\n" + commandList.String() +
		"Do not output anything other than the JSON object.\n" +
		"Session history:\n" + history
}

// sanitizeJSON fixes common invalid JSON produced by LLMs:
// - \<newline> (backslash continuation) -> <newline>
// - \' (invalid escape) -> '
// - strips markdown code fences wrapping JSON
func sanitizeJSON(s string) string {
	// Strip markdown code fences (```json ... ```).
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
		s = strings.TrimSpace(s)
	}
	// Replace backslash-continuation (\ followed by newline) with just newline.
	s = strings.ReplaceAll(s, "\\\n", "\n")
	s = strings.ReplaceAll(s, "\\\r\n", "\r\n")
	// Replace invalid \' escape with literal '.
	s = strings.ReplaceAll(s, `\'`, `'`)
	return s
}

// parseConverseOutput extracts command(s) from the Bedrock Converse response.
func parseConverseOutput(output *bedrockruntime.ConverseOutput) []string {
	msg, ok := output.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		logf("Unexpected output type received.")
		return nil
	}

	for _, content := range msg.Value.Content {
		if textBlock, ok := content.(*types.ContentBlockMemberText); ok {
			logf("[llm] Raw response: %s", textBlock.Value)
			raw := sanitizeJSON(textBlock.Value)
			var resp llmResponse
			if err := json.Unmarshal([]byte(raw), &resp); err != nil {
				logf("[llm] Failed to unmarshal response JSON: %v", err)
				// Return a respond command so the agent loop terminates gracefully.
				return []string{CommandRespond + " I encountered a response parsing error. Please try again."}
			}
			return resp.parseCommands()
		}
	}

	return nil
}
