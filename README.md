# fastllm

A fast CLI coding agent that keeps your entire workspace in memory.

## Why it's fast

Traditional coding agents shell out to the filesystem for every read, search, and write. fastllm loads all file paths and contents into memory at startup and keeps them in sync as the agent works. The LLM only receives file contents when it explicitly requests them, and file metadata (paths + line counts) is always available in the prompt context — no round-trips needed.

## How it works

1. **Startup** — walks the project directory, indexes every text file into an in-memory map of `path → content`.
2. **Prompt** — sends the user's request plus a map of `path → line count` so the LLM knows what exists and how large each file is.
3. **Agent loop** — the LLM issues tool commands (`findfile`, `readlines`, `writefile`, etc.), fastllm executes them against the in-memory map, and feeds results back. Repeats until the LLM responds.
4. **Disk sync** — write commands update both the in-memory map and the filesystem atomically.

## Tool commands

| Command | Description |
|---------|-------------|
| `readfile <path>` | Read entire file (prefer for small files < 50 lines) |
| `readlines <path> <start> <end>` | Read specific line range (1-indexed, inclusive) |
| `findfile <pattern>` | Search all files for a text pattern |
| `findfile <pattern> <path>` | Search within a single file |
| `writefile <path>\n<content>` | Create or overwrite a file |
| `writelines <path> <start> <end>\n<content>` | Replace a line range |
| `respond <message>` | Return final answer to the user |

## Setup

Requires Go 1.25+ and a Bedrock API key.

```bash
# Set environment variables
export BEDROCK_API_KEY=your-key
export AWS_REGION=us-east-1

# Build and run
go build -o fastllm .
./fastllm
```

## Architecture

```
main.go       — Bubble Tea UI (input, viewport, spinner)
llm.go        — Bedrock API client, system prompt, agent loop
commands.go   — tool command parsing and execution
files.go      — workspace scanner (BuildPathMap)
log.go        — deferred file logger
```