package contentcheck

import "strings"

// CheckHTML scans content for raw HTML tags outside fenced code blocks.
// YAML frontmatter (--- delimited at file start) is also skipped.
func CheckHTML(content string, file string) []Violation {
	var violations []Violation
	lines := strings.Split(content, "\n")

	inFrontmatter := false
	frontmatterDone := false
	inFence := false
	var fenceChar byte
	var fenceLen int

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// YAML frontmatter: only at start of file
		if i == 0 && trimmed == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter && !frontmatterDone {
			if trimmed == "---" || trimmed == "..." {
				inFrontmatter = false
				frontmatterDone = true
			}
			continue
		}

		if !inFence {
			if opened, char, length := parseFenceOpen(line); opened {
				inFence = true
				fenceChar = char
				fenceLen = length
				continue
			}
		} else {
			if isFenceClose(trimmed, fenceChar, fenceLen) {
				inFence = false
			}
			continue
		}

		if col := findHTMLTag(line); col >= 0 {
			violations = append(violations, Violation{
				File:   file,
				Line:   i + 1,
				Column: col + 1,
				Kind:   KindHTML,
				Reason: "raw HTML tag outside fenced code block",
			})
		}
	}

	return violations
}

// parseFenceOpen checks if a line opens a fenced code block.
// The raw (untrimmed) line is needed to enforce the 0-3 spaces indent rule.
func parseFenceOpen(line string) (bool, byte, int) {
	// Count leading spaces -- CommonMark allows 0-3
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return false, 0, 0
	}

	rest := line[indent:]
	if len(rest) < 3 {
		return false, 0, 0
	}

	char := rest[0]
	if char != '`' && char != '~' {
		return false, 0, 0
	}

	count := 0
	for count < len(rest) && rest[count] == char {
		count++
	}
	if count < 3 {
		return false, 0, 0
	}

	// Backtick fences cannot have backticks in the info string
	if char == '`' && strings.ContainsRune(rest[count:], '`') {
		return false, 0, 0
	}

	return true, char, count
}

// isFenceClose checks if a trimmed line closes the current fence.
func isFenceClose(trimmed string, fenceChar byte, fenceLen int) bool {
	if len(trimmed) == 0 {
		return false
	}
	if trimmed[0] != fenceChar {
		return false
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] != fenceChar {
			return false
		}
	}
	return len(trimmed) >= fenceLen
}

// findHTMLTag returns the byte offset of the first HTML-like tag in the line,
// or -1 if none found. Matches <letter or </letter patterns.
func findHTMLTag(line string) int {
	for i := 0; i < len(line)-1; i++ {
		if line[i] != '<' {
			continue
		}
		next := i + 1
		if next < len(line) && line[next] == '/' {
			next++
		}
		if next < len(line) && isASCIILetter(line[next]) {
			return i
		}
	}
	return -1
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
