package contentcheck

import "testing"

func TestFrontmatterSafeContent(t *testing.T) {
	content := "---\ntitle: Code Review\nauthor: alice\ntags:\n  - review\n---\n# Prompt\n"
	violations := CheckFrontmatter(content, "test.md")
	if len(violations) > 0 {
		t.Errorf("safe frontmatter should be accepted, got: %v", violations)
	}
}

func TestFrontmatterNoFrontmatter(t *testing.T) {
	content := "# Just a heading\nNo frontmatter here.\n"
	violations := CheckFrontmatter(content, "test.md")
	if len(violations) > 0 {
		t.Errorf("no frontmatter should be accepted, got: %v", violations)
	}
}

func TestFrontmatterAnchor(t *testing.T) {
	content := "---\nbase: &base\n  key: value\n---\n# Prompt\n"
	violations := CheckFrontmatter(content, "test.md")
	if len(violations) == 0 {
		t.Fatal("expected violation for YAML anchor")
	}
	if violations[0].Kind != KindFrontmatter {
		t.Errorf("kind: got %q", violations[0].Kind)
	}
}

func TestFrontmatterAlias(t *testing.T) {
	content := "---\nref: *base\n---\n# Prompt\n"
	violations := CheckFrontmatter(content, "test.md")
	if len(violations) == 0 {
		t.Fatal("expected violation for YAML alias")
	}
}

func TestFrontmatterMergeKey(t *testing.T) {
	content := "---\n<<: *defaults\nname: test\n---\n# Prompt\n"
	violations := CheckFrontmatter(content, "test.md")
	if len(violations) == 0 {
		t.Fatal("expected violation for merge key")
	}
}

func TestFrontmatterInvalidYAML(t *testing.T) {
	content := "---\n: invalid: yaml: here:\n---\n# Prompt\n"
	violations := CheckFrontmatter(content, "test.md")
	if len(violations) == 0 {
		t.Fatal("expected violation for invalid YAML")
	}
}

func TestFrontmatterSafeAmpersand(t *testing.T) {
	content := "---\ndescription: \"C++ & Go integration\"\ntags:\n  - review\n---\n# Prompt\n"
	violations := CheckFrontmatter(content, "test.md")
	if len(violations) > 0 {
		t.Errorf("ampersand in value should be accepted, got: %v", violations)
	}
}

func TestFrontmatterSafeAsterisk(t *testing.T) {
	content := "---\npattern: \"glob: *.md\"\n---\n# Prompt\n"
	violations := CheckFrontmatter(content, "test.md")
	if len(violations) > 0 {
		t.Errorf("asterisk in value should be accepted, got: %v", violations)
	}
}
