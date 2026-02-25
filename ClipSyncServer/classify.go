package main

import (
	"regexp"
	"strings"
)

// ============================================================================
// Content classification
// ============================================================================

var urlPattern = regexp.MustCompile(`^https?://\S+$`)

// Code indicators: keywords and syntax patterns typical of programming languages.
var codeIndicators = []string{
	"func ", "def ", "class ", "import ", "package ",
	"const ", "var ", "let ", "return ",
	"if (", "if(", "for (", "for(",
	"=>", "->", ":=", "!=", "==",
	"#!/", "<?php", "<html", "<!DOCTYPE",
	"SELECT ", "INSERT ", "UPDATE ", "CREATE TABLE",
	"console.log", "fmt.Print", "System.out",
}

// detectCategory analyses clipboard content and returns "url", "code", or "text".
func detectCategory(content string) string {
	trimmed := strings.TrimSpace(content)

	// 1. URL detection — single-line content that looks like a URL
	if !strings.Contains(trimmed, "\n") && urlPattern.MatchString(trimmed) {
		return "url"
	}

	// 2. Code detection
	// Check for known code keywords/syntax
	for _, indicator := range codeIndicators {
		if strings.Contains(content, indicator) {
			return "code"
		}
	}

	// Multi-line content with consistent indentation (tabs or 2+ spaces) suggests code
	lines := strings.Split(trimmed, "\n")
	if len(lines) >= 3 {
		indentedLines := 0
		for _, line := range lines {
			if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "  ") {
				indentedLines++
			}
		}
		// If > 40% of lines are indented, likely code
		if float64(indentedLines)/float64(len(lines)) > 0.4 {
			return "code"
		}
	}

	// Check for bracket-heavy content (JSON, code blocks)
	braces := strings.Count(content, "{") + strings.Count(content, "}")
	if braces >= 4 {
		return "code"
	}

	// 3. Default: plain text
	return "text"
}
