package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// models
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

// helper functions
func getLatestCommit() (string, error) {
	cmd := exec.Command("git", "show", "--stat", "--patch", "HEAD")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		// If not in a git repo or git fails, return a clear error
		return "", fmt.Errorf("failed to run git show: %w", err)
	}
	commitText := strings.TrimSpace(string(out))
	if commitText == "" {
		return "", fmt.Errorf("empty git show output")
	}
	return commitText, nil
}

func explainCommit(commitText string) (string, error) {
	model := os.Getenv("LMSTUDIO_MODEL")
	if model == "" {
		return "", fmt.Errorf("LMSTUDIO_MODEL is not set")
	}

	baseURL := os.Getenv("LMSTUDIO_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:1234/v1"
	}

	apiKey := os.Getenv("LMSTUDIO_API_KEY")
	if apiKey == "" {
		apiKey = "lm-studio"
	}

	temp := 0.2
	if tStr := os.Getenv("EXPLAIN_TEMPERATURE"); tStr != "" {
		if tParsed, err := strconv.ParseFloat(tStr, 64); err == nil {
			temp = tParsed
		}
	}

	systemPrompt := strings.TrimSpace(`
You are a senior software engineer explaining a Git commit to a teammate.

Rules:
- Give a short high-level summary first (1–3 bullet points).
- Then describe the main code changes grouped by concern (e.g. "API", "UI", "tests").
- Explain WHY the changes might have been made (best-effort inference).
- Keep it concise but clear. No more than about 20 lines total.
`)

	userPrompt := fmt.Sprintf(strings.TrimSpace(`
Here is the latest commit on the current branch:

%s

Explain this commit following the rules.
`), commitText)

	reqBody := chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: temp,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	url := strings.TrimRight(baseURL, "/") + "/chat/completions"

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 || chatResp.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("invalid response: missing choices[0].message.content")
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

// main function
func main() {
	// Simple flag: --raw just prints the raw git show
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Println(`explain-commit - explain the latest Git commit using LM Studio

Usage:
  explain-commit [--raw]

Options:
  --raw   Print the raw git show output and exit`)
		return
	}

	fmt.Println("🔍 Reading latest commit (git show HEAD)...")
	commitText, err := getLatestCommit()
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	if len(os.Args) > 1 && os.Args[1] == "--raw" {
		fmt.Println("----- RAW COMMIT -----")
		fmt.Println(commitText)
		return
	}

	fmt.Printf("✓ Got commit (%d characters)\n", len(commitText))

	fmt.Println("🧠 Asking LM Studio to explain the commit...")
	explanation, err := explainCommit(commitText)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	fmt.Println("\n📄 Explanation:")
	fmt.Println(explanation)
}
