# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A CLI tool that explains the latest Git commit using LM Studio's local LLM API. It fetches the most recent commit details via `git show` and sends them to a locally running LM Studio instance for human-readable explanation.

## Build and Run Commands

```bash
# Build the binary
go build -o explain-commit .

# Run directly
go run .

# Run with raw commit output (no LLM explanation)
go run . --raw
```

## Architecture

- **main.go**: Entry point and core logic
  - `getLatestCommit()`: Runs `git show --stat --patch HEAD` to get commit details
  - `explainCommit()`: Sends commit text to LM Studio API for explanation
  - Uses OpenAI-compatible chat completions API (`/v1/chat/completions`)

- **services/services.go**: LM Studio connectivity
  - `IsLMStudioRunning()`: Health check via `/v1/models` endpoint

## Configuration

- LM Studio API: Hardcoded to `http://localhost:1234/v1` with model `qwen/qwen3-4b-2507`
- `EXPLAIN_TEMPERATURE` env var: Controls LLM temperature (default: 0.2)

## Dependencies

Requires LM Studio running locally on port 1234 with a loaded model.
