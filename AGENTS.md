# Project objectives

The objective of this project is to create a CLI coding agent that is faster than existing agents. The main
solution to achieve this is to keep every file path in memory as well as file contents, file paths are always up to date and file contents are updated when the agent is working on a file. This way we can avoid the latency of file system operations and we can also avoid the latency of LLM calls by keeping the context of the current file in memory. The file contents are only sent to the LLM when the agent requests the file contents.

# Instructions

The /bubbletea directory contains the bubbletea source code and should always be searched when making bubbletea changes.

Always prioritize performance and speed of response to the user.

Use modern golang features.

Always read fastllm.log when debugging.

When adding new logic, add unit tests.
