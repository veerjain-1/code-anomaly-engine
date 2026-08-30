// Package parser extracts function-level code snippets from unified diffs.
// It uses heuristic boundary detection to isolate individual functions
// from multi-function diffs for granular anomaly analysis.
package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Snippet represents a function-level code extract from a diff.
type Snippet struct {
	Code      string `json:"code"`
	Repo      string `json:"repo"`
	CommitSHA string `json:"commit_sha"`
	FilePath  string `json:"file_path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Timestamp int64  `json:"timestamp"`
}

// hunkHeaderRegex matches unified diff hunk headers: @@ -a,b +c,d @@ optional context
var hunkHeaderRegex = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@(.*)$`)

// funcBoundaryRegex matches common function definitions across C/C++/Go/Rust/Python/JS/Java
var funcBoundaryRegex = regexp.MustCompile(
	`(?m)^(?:` +
		// C/C++/Java: return_type function_name(
		`(?:(?:static|inline|virtual|override|const|unsigned|signed|void|int|long|char|float|double|bool|auto)\s+)*\w+\s+\w+\s*\(` +
		// Go: func name(
		`|func\s+(?:\([^)]*\)\s*)?\w+\s*\(` +
		// Rust: fn name(
		`|(?:pub\s+)?(?:async\s+)?fn\s+\w+` +
		// Python: def name(
		`|def\s+\w+\s*\(` +
		// JS/TS: function name( / const name = ( / name(
		`|(?:export\s+)?(?:async\s+)?function\s+\w+\s*\(` +
		`)`,
)

// ExtractSnippets parses a unified diff and returns function-level code snippets.
func ExtractSnippets(diff string, repo string, commitSHA string) []Snippet {
	if diff == "" {
		return nil
	}

	var snippets []Snippet
	now := time.Now().UnixMilli()

	// Split into per-file diffs
	fileDiffs := splitFileDiffs(diff)

	for _, fileDiff := range fileDiffs {
		filePath := extractFilePath(fileDiff)
		if filePath == "" {
			continue
		}

		// Extract hunks
		hunks := extractHunks(fileDiff)
		for _, hunk := range hunks {
			// Try to extract function-level snippets from the hunk
			functions := extractFunctions(hunk.addedLines, hunk.startLine)
			if len(functions) == 0 {
				// If no function boundaries found, use the whole hunk
				if len(hunk.addedLines) > 0 {
					snippets = append(snippets, Snippet{
						Code:      strings.Join(hunk.addedLines, "\n"),
						Repo:      repo,
						CommitSHA: commitSHA,
						FilePath:  filePath,
						StartLine: hunk.startLine,
						EndLine:   hunk.startLine + len(hunk.addedLines) - 1,
						Timestamp: now,
					})
				}
			} else {
				for _, fn := range functions {
					snippets = append(snippets, Snippet{
						Code:      fn.code,
						Repo:      repo,
						CommitSHA: commitSHA,
						FilePath:  filePath,
						StartLine: fn.startLine,
						EndLine:   fn.endLine,
						Timestamp: now,
					})
				}
			}
		}
	}

	return snippets
}

type hunk struct {
	startLine  int
	addedLines []string
}

type functionBlock struct {
	code      string
	startLine int
	endLine   int
}

// splitFileDiffs splits a multi-file unified diff into per-file chunks.
func splitFileDiffs(diff string) []string {
	var result []string
	var current strings.Builder
	lines := strings.Split(diff, "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// extractFilePath gets the file path from a unified diff header.
func extractFilePath(fileDiff string) string {
	lines := strings.Split(fileDiff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+++ b/") {
			return strings.TrimPrefix(line, "+++ b/")
		}
		if strings.HasPrefix(line, "+++ ") {
			path := strings.TrimPrefix(line, "+++ ")
			// Remove leading "b/" prefix common in git diffs
			path = strings.TrimPrefix(path, "b/")
			return path
		}
	}
	return ""
}

// extractHunks parses hunk headers and collects added lines per hunk.
func extractHunks(fileDiff string) []hunk {
	var hunks []hunk
	lines := strings.Split(fileDiff, "\n")

	var current *hunk
	lineNum := 0

	for _, line := range lines {
		if matches := hunkHeaderRegex.FindStringSubmatch(line); matches != nil {
			if current != nil && len(current.addedLines) > 0 {
				hunks = append(hunks, *current)
			}
			startLine, _ := strconv.Atoi(matches[2]) // +side line number
			current = &hunk{startLine: startLine}
			lineNum = startLine
			continue
		}

		if current == nil {
			continue
		}

		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			// Added line — strip the leading "+"
			current.addedLines = append(current.addedLines, strings.TrimPrefix(line, "+"))
			lineNum++
		} else if strings.HasPrefix(line, "-") {
			// Removed line — don't increment line number
			continue
		} else {
			// Context line
			lineNum++
		}
	}

	if current != nil && len(current.addedLines) > 0 {
		hunks = append(hunks, *current)
	}

	return hunks
}

// extractFunctions attempts to split a block of code into function-level chunks.
func extractFunctions(lines []string, baseLineNum int) []functionBlock {
	if len(lines) == 0 {
		return nil
	}

	code := strings.Join(lines, "\n")
	matches := funcBoundaryRegex.FindAllStringIndex(code, -1)
	if len(matches) == 0 {
		return nil
	}

	var functions []functionBlock
	for i, match := range matches {
		start := match[0]
		var end int
		if i+1 < len(matches) {
			end = matches[i+1][0]
		} else {
			end = len(code)
		}

		funcCode := strings.TrimSpace(code[start:end])
		if len(funcCode) < 10 { // Skip trivially small fragments
			continue
		}

		// Calculate line numbers
		startLine := baseLineNum + strings.Count(code[:start], "\n")
		endLine := startLine + strings.Count(funcCode, "\n")

		functions = append(functions, functionBlock{
			code:      funcCode,
			startLine: startLine,
			endLine:   endLine,
		})
	}

	return functions
}

// FormatSnippetID creates a unique identifier for a code snippet.
func FormatSnippetID(repo, commitSHA, filePath string, startLine int) string {
	return fmt.Sprintf("%s/%s/%s#L%d", repo, commitSHA[:8], filePath, startLine)
}
