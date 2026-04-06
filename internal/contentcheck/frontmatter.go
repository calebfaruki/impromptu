package contentcheck

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// CheckFrontmatter validates YAML frontmatter for dangerous constructs.
// Rejects anchors, aliases, and merge keys by walking the YAML AST.
func CheckFrontmatter(content string, file string) []Violation {
	fm := extractFrontmatter(content)
	if fm == "" {
		return nil
	}

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(fm), &node); err != nil {
		return []Violation{{
			File:   file,
			Kind:   KindFrontmatter,
			Reason: fmt.Sprintf("invalid YAML frontmatter: %v", err),
		}}
	}

	var violations []Violation
	walkYAMLNode(&node, file, &violations)
	return violations
}

func walkYAMLNode(node *yaml.Node, file string, violations *[]Violation) {
	if node == nil {
		return
	}

	if node.Kind == yaml.AliasNode {
		*violations = append(*violations, Violation{
			File:   file,
			Line:   node.Line + 1, // +1 for opening ---
			Kind:   KindFrontmatter,
			Reason: "YAML aliases are not allowed in frontmatter",
		})
		return
	}

	if node.Anchor != "" {
		*violations = append(*violations, Violation{
			File:   file,
			Line:   node.Line + 1,
			Kind:   KindFrontmatter,
			Reason: "YAML anchors are not allowed in frontmatter",
		})
		return
	}

	if node.Tag == "!!merge" {
		*violations = append(*violations, Violation{
			File:   file,
			Line:   node.Line + 1,
			Kind:   KindFrontmatter,
			Reason: "YAML merge keys are not allowed in frontmatter",
		})
		return
	}

	for _, child := range node.Content {
		walkYAMLNode(child, file, violations)
	}
}

func extractFrontmatter(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}

	for i := 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "---" || trimmed == "..." {
			return strings.Join(lines[1:i], "\n")
		}
	}
	return ""
}
