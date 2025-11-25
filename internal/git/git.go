package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// GetLatestCommit returns the full details of the HEAD commit.
func GetLatestCommit() (string, error) {
	cmd := exec.Command("git", "show", "--stat", "--patch", "HEAD")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run git show: %w", err)
	}
	commitText := strings.TrimSpace(string(out))
	if commitText == "" {
		return "", fmt.Errorf("empty git show output")
	}
	return commitText, nil
}
