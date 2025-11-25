package main

import (
	"fmt"
	"log"
	"os"

	"github.com/yourname/explain-commit/internal/git"
	"github.com/yourname/explain-commit/internal/llm"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Println(`explain-commit - explain the latest Git commit using LM Studio

Usage:
  explain-commit [--raw]

Options:
  --raw   Print the raw git show output and exit`)
		return
	}

	fmt.Println("🔍 Reading latest commit (git show HEAD)...")
	commitText, err := git.GetLatestCommit()
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
	explanation, err := llm.ExplainCommit(commitText)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	fmt.Println("\n📄 Explanation:")
	fmt.Println(explanation)
}
