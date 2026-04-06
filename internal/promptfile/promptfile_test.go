package promptfile

import (
	"strings"
	"testing"
)

// --- Path validation ---

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid simple", "subdir", false},
		{"valid nested", "nested/subdir", false},
		{"reject dotdot", "../escape", true},
		{"reject dotdot mid", "foo/../bar", true},
		{"reject absolute slash", "/etc/prompts", true},
		{"reject home tilde", "~/.ssh", true},
		{"reject backslash", "sub\\dir", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.path)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// --- Parse tests ---

func TestParseGitTag(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\ncoder = {git = \"https://github.com/alice/coder\", tag = \"v1\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	src := pf.Prompts["coder"]
	if src.Kind != SourceGit {
		t.Errorf("kind: got %q", src.Kind)
	}
	if src.Git != "https://github.com/alice/coder" {
		t.Errorf("git: got %q", src.Git)
	}
	if src.Tag != "v1" {
		t.Errorf("tag: got %q", src.Tag)
	}
}

func TestParseGitBranch(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\ndev = {git = \"https://github.com/alice/coder\", branch = \"main\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if pf.Prompts["dev"].Branch != "main" {
		t.Errorf("branch: got %q", pf.Prompts["dev"].Branch)
	}
}

func TestParseGitCommit(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\npinned = {git = \"https://github.com/alice/coder\", commit = \"a1b2c3d\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if pf.Prompts["pinned"].Commit != "a1b2c3d" {
		t.Errorf("commit: got %q", pf.Prompts["pinned"].Commit)
	}
}

func TestParseGitWithPath(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nwriter = {git = \"https://github.com/alice/prompts\", tag = \"v1\", path = \"writer\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if pf.Prompts["writer"].Path != "writer" {
		t.Errorf("path: got %q", pf.Prompts["writer"].Path)
	}
}

func TestParseGitSSH(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\ninternal = {git = \"git@github.com:alice/coder.git\", tag = \"v2\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if pf.Prompts["internal"].Git != "git@github.com:alice/coder.git" {
		t.Errorf("git: got %q", pf.Prompts["internal"].Git)
	}
}

func TestParseOCITag(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nreviewer = {oci = \"ghcr.io/alice/reviewer\", tag = \"v1\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	src := pf.Prompts["reviewer"]
	if src.Kind != SourceOCI {
		t.Errorf("kind: got %q", src.Kind)
	}
	if src.OCITag != "v1" {
		t.Errorf("tag: got %q", src.OCITag)
	}
}

func TestParseOCIDigest(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\npinned = {oci = \"ghcr.io/alice/reviewer\", digest = \"sha256:abc123\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if pf.Prompts["pinned"].Digest != "sha256:abc123" {
		t.Errorf("digest: got %q", pf.Prompts["pinned"].Digest)
	}
}

func TestParseInline(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nclaude = {git = \"https://github.com/alice/claude-md\", tag = \"v1\", inline = true}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !pf.Prompts["claude"].Inline {
		t.Error("expected inline = true")
	}
}

func TestRejectGitAndOCI(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nbad = {git = \"url\", oci = \"ref\", tag = \"v1\"}\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for git + oci")
	}
	if !strings.Contains(err.Error(), "both git and oci") {
		t.Errorf("error should mention both: %v", err)
	}
}

func TestRejectTagAndBranch(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nbad = {git = \"url\", tag = \"v1\", branch = \"main\"}\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for tag + branch")
	}
}

func TestRejectPathOnOCI(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nbad = {oci = \"ref\", tag = \"v1\", path = \"sub\"}\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for path on OCI")
	}
}

func TestRejectOldShortForm(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\ncoder = \"alice/coder@1\"\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for old short form")
	}
	if !strings.Contains(err.Error(), "no longer supported") {
		t.Errorf("error should give migration guidance: %v", err)
	}
}

func TestRejectPathTraversal(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nbad = {git = \"url\", tag = \"v1\", path = \"../escape\"}\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRejectAbsolutePath(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nbad = {git = \"url\", tag = \"v1\", path = \"/etc/prompts\"}\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRejectBackslashPath(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nbad = {git = \"url\", tag = \"v1\", path = \"sub\\\\dir\"}\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- SourceFromFlags ---

func TestSourceFromFlagsGitTag(t *testing.T) {
	src, err := SourceFromFlags("https://github.com/alice/coder", "", "v1", "", "", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if src.Kind != SourceGit || src.Tag != "v1" {
		t.Errorf("got %+v", src)
	}
}

func TestSourceFromFlagsOCIDigest(t *testing.T) {
	src, err := SourceFromFlags("", "ghcr.io/alice/reviewer", "", "", "", "sha256:abc", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if src.Kind != SourceOCI || src.Digest != "sha256:abc" {
		t.Errorf("got %+v", src)
	}
}

func TestSourceFromFlagsMissingVersion(t *testing.T) {
	_, err := SourceFromFlags("https://github.com/alice/coder", "", "", "", "", "", "", false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSourceFromFlagsBothGitAndOCI(t *testing.T) {
	_, err := SourceFromFlags("git-url", "oci-ref", "v1", "", "", "", "", false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSourceFromFlagsInline(t *testing.T) {
	src, err := SourceFromFlags("https://github.com/alice/claude-md", "", "v1", "", "", "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !src.Inline {
		t.Error("expected inline")
	}
}

// --- Alias ---

func TestAliasFromGitURL(t *testing.T) {
	if a := AliasFromSource(Source{Kind: SourceGit, Git: "https://github.com/alice/coder"}); a != "coder" {
		t.Errorf("got %q", a)
	}
}

func TestAliasFromGitSuffix(t *testing.T) {
	if a := AliasFromSource(Source{Kind: SourceGit, Git: "git@github.com:alice/coder.git"}); a != "coder" {
		t.Errorf("got %q", a)
	}
}

func TestAliasFromOCI(t *testing.T) {
	if a := AliasFromSource(Source{Kind: SourceOCI, OCI: "ghcr.io/alice/reviewer"}); a != "reviewer" {
		t.Errorf("got %q", a)
	}
}

// --- Write ---

func TestAddSourceAndRemove(t *testing.T) {
	pf := &Promptfile{Version: 1, Prompts: map[string]Source{}}
	if err := pf.AddSource("coder", Source{Kind: SourceGit, Git: "url", Tag: "v1"}); err != nil {
		t.Fatal(err)
	}
	if err := pf.RemoveEntry("coder"); err != nil {
		t.Fatal(err)
	}
	if len(pf.Prompts) != 0 {
		t.Error("not removed")
	}
}

func TestRoundTrip(t *testing.T) {
	original := &Promptfile{
		Version: 1,
		Prompts: map[string]Source{
			"coder":    {Kind: SourceGit, Git: "https://github.com/alice/coder", Tag: "v1"},
			"reviewer": {Kind: SourceOCI, OCI: "ghcr.io/alice/reviewer", OCITag: "v2"},
		},
	}
	data, err := original.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Prompts) != 2 {
		t.Fatalf("got %d", len(parsed.Prompts))
	}
	if parsed.Prompts["coder"].Tag != "v1" {
		t.Error("coder tag mismatch")
	}
	if parsed.Prompts["reviewer"].OCITag != "v2" {
		t.Error("reviewer tag mismatch")
	}
}

func TestRoundTripInline(t *testing.T) {
	original := &Promptfile{
		Version: 1,
		Prompts: map[string]Source{
			"claude": {Kind: SourceGit, Git: "https://github.com/alice/claude-md", Tag: "v1", Inline: true},
		},
	}
	data, err := original.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Prompts["claude"].Inline {
		t.Error("inline lost in round-trip")
	}
}

func TestEmptyPrompts(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(pf.Prompts) != 0 {
		t.Errorf("expected 0, got %d", len(pf.Prompts))
	}
}
