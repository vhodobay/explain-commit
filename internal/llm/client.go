package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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

// isLMSCLIAvailable checks if the `lms` CLI tool is available.
func isLMSCLIAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "lms", "version")
	return cmd.Run() == nil
}

// isModelLoaded checks if the specified model is currently loaded using `lms ps`.
func isModelLoaded(modelID string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "lms", "ps")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), modelID)
}

// loadModel loads a model using `lms load`.
func loadModel(modelID string) error {
	fmt.Printf("Loading model: %s...\n", modelID)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "lms", "load", modelID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to load model: %s", string(output))
	}
	fmt.Println("Model loaded successfully")
	return nil
}

// startServerWithCLI starts the LM Studio server using `lms server start`.
func startServerWithCLI() error {
	fmt.Println("Starting LM Studio server with `lms server start`...")
	cmd := exec.Command("lms", "server", "start")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start server: %s", string(output))
	}
	return nil
}

// waitForServer waits for the LM Studio server to become accessible.
func waitForServer(baseURL string, timeout time.Duration) error {
	fmt.Println("Waiting for LM Studio server...")
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if IsRunning(baseURL) {
			fmt.Println("LM Studio server is ready")
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timed out waiting for LM Studio server at %s", baseURL)
}

// StartLMStudio ensures the LM Studio server is running and the model is loaded.
// It prefers the headless `lms` CLI over starting the GUI application.
func StartLMStudio() error {
	if IsRunning(defaultBaseURL) {
		// Server is running, check if model is loaded
		if isLMSCLIAvailable() {
			if !isModelLoaded(defaultModel) {
				fmt.Println("Model is not loaded")
				if err := loadModel(defaultModel); err != nil {
					return err
				}
			}
		}
		return nil
	}

	fmt.Println("LM Studio server is not running")

	// Try to use `lms` CLI first (headless mode)
	if isLMSCLIAvailable() {
		if err := startServerWithCLI(); err != nil {
			return err
		}
		if err := waitForServer(defaultBaseURL, 30*time.Second); err != nil {
			return err
		}
		// Load the model
		if !isModelLoaded(defaultModel) {
			if err := loadModel(defaultModel); err != nil {
				return err
			}
		}
		return nil
	}

	return fmt.Errorf("LM Studio is not running and `lms` CLI is not available; please install LM Studio CLI or start the server manually")
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
