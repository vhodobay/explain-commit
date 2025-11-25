package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "http://localhost:1234/v1"
	defaultModel   = "qwen/qwen3-4b-2507"
	defaultAPIKey  = "lm-studio"
)

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

// IsRunning checks whether the LM Studio API is reachable.
func IsRunning(baseURL string) bool {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	client := &http.Client{Timeout: 5 * time.Second}
	endpoint := strings.TrimRight(baseURL, "/") + "/models"
	resp, err := client.Get(endpoint)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// If not running, start the lm studio server
func StartLMStudio() error {
	if IsRunning(defaultBaseURL) {
		return nil
	}

	var (
		cmd *exec.Cmd
	)

	if envCmd := strings.TrimSpace(os.Getenv("LM_STUDIO_CMD")); envCmd != "" {
		parts := strings.Fields(envCmd)
		if len(parts) == 0 {
			return fmt.Errorf("LM_STUDIO_CMD is set but empty")
		}
		cmd = exec.Command(parts[0], parts[1:]...)
	} else {
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", "-a", "LM Studio")
		case "windows":
			return fmt.Errorf("LM Studio is not running; start it manually on Windows")
		default:
			cmd = exec.Command("lmstudio")
		}
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start LM Studio: %w", err)
	}

	// Avoid zombie processes while still letting LM Studio run independently.
	go func() {
		_ = cmd.Wait()
	}()

	readyTimeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if IsRunning(defaultBaseURL) {
				return nil
			}
		case <-readyTimeout:
			return fmt.Errorf("timed out waiting for LM Studio to become reachable at %s", defaultBaseURL)
		}
	}
}

// ExplainCommit sends the commit text to LM Studio and returns an explanation.
// If LM Studio is not running, it will attempt to start it automatically.
func ExplainCommit(commitText string) (string, error) {
	baseURL := defaultBaseURL
	model := defaultModel
	apiKey := defaultAPIKey

	if err := StartLMStudio(); err != nil {
		return "", err
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

	client := &http.Client{Timeout: 60 * time.Second}
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
