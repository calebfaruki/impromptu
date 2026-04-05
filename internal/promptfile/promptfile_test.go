package promptfile

import (
	"strings"
	"testing"
)

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

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{"latest", "latest", false},
		{"major only", "2", false},
		{"major only single", "1", false},
		{"exact semver", "2.1.0", false},
		{"exact semver zeros", "0.0.1", false},
		{"digest pin", "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", false},
		{"reject empty", "", true},
		{"reject bare word", "stable", true},
		{"reject partial semver", "2.1", true},
		{"reject four part", "1.2.3.4", true},
		{"reject short digest", "sha256:abc", true},
		{"reject uppercase digest", "sha256:2CF24DBA5FB0A30E26E83B2AC5B9E29E1B161E5C1FA7425E73043362938B9824", true},
		{"reject md5 prefix", "md5:abc123", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersion(tt.version)
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

func TestParseShortForm(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\ncoder = \"alice/coder@1\"\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	src, ok := pf.Prompts["coder"]
	if !ok {
		t.Fatal("missing prompt 'coder'")
	}
	if src.Kind != SourceRegistry {
		t.Errorf("kind: got %q, want %q", src.Kind, SourceRegistry)
	}
	if src.Ref != "alice/coder@1" {
		t.Errorf("ref: got %q, want %q", src.Ref, "alice/coder@1")
	}
}

func TestParseLongForm(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nreviewer = {ref = \"bob/code-review@2.1.0\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	src := pf.Prompts["reviewer"]
	if src.Kind != SourceRegistry {
		t.Errorf("kind: got %q, want %q", src.Kind, SourceRegistry)
	}
	if src.Ref != "bob/code-review@2.1.0" {
		t.Errorf("ref: got %q", src.Ref)
	}
}

func TestParseGitTag(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\ninternal = {git = \"https://github.com/org/repo\", tag = \"v1\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	src := pf.Prompts["internal"]
	if src.Kind != SourceGit {
		t.Errorf("kind: got %q", src.Kind)
	}
	if src.Git != "https://github.com/org/repo" {
		t.Errorf("git: got %q", src.Git)
	}
	if src.Tag != "v1" {
		t.Errorf("tag: got %q", src.Tag)
	}
}

func TestParseGitBranch(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nnightly = {git = \"https://github.com/org/repo\", branch = \"main\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if pf.Prompts["nightly"].Branch != "main" {
		t.Errorf("branch: got %q", pf.Prompts["nightly"].Branch)
	}
}

func TestParseGitCommit(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\npinned = {git = \"https://github.com/org/repo\", commit = \"abc123\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if pf.Prompts["pinned"].Commit != "abc123" {
		t.Errorf("commit: got %q", pf.Prompts["pinned"].Commit)
	}
}

func TestParseGitWithPath(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nsub = {git = \"https://github.com/org/repo\", tag = \"v1\", path = \"review\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if pf.Prompts["sub"].Path != "review" {
		t.Errorf("path: got %q", pf.Prompts["sub"].Path)
	}
}

func TestParseOCITag(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nimg = {oci = \"ghcr.io/org/prompt\", tag = \"v1\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	src := pf.Prompts["img"]
	if src.Kind != SourceOCI {
		t.Errorf("kind: got %q", src.Kind)
	}
	if src.OCITag != "v1" {
		t.Errorf("tag: got %q", src.OCITag)
	}
}

func TestParseOCIDigest(t *testing.T) {
	digest := "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	data := []byte("version = 1\n\n[prompts]\npinned = {oci = \"ghcr.io/org/prompt\", digest = \"" + digest + "\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if pf.Prompts["pinned"].Digest != digest {
		t.Errorf("digest: got %q", pf.Prompts["pinned"].Digest)
	}
}

func TestParsePrivateRegistry(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\ncorp = {registry = \"https://prompts.internal.co\", ref = \"team/deploy@latest\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	src := pf.Prompts["corp"]
	if src.Kind != SourcePrivate {
		t.Errorf("kind: got %q", src.Kind)
	}
	if src.Registry != "https://prompts.internal.co" {
		t.Errorf("registry: got %q", src.Registry)
	}
}

func TestParseMissingVersion(t *testing.T) {
	data := []byte("[prompts]\ncoder = \"alice/coder@1\"\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for missing version")
	}
}

func TestParseEmptyPrompts(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(pf.Prompts) != 0 {
		t.Errorf("expected 0 prompts, got %d", len(pf.Prompts))
	}
}

func TestParseRejectPathTraversal(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nbad = {git = \"https://github.com/org/repo\", tag = \"v1\", path = \"../escape\"}\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

// --- Write tests ---

func TestAddEntry(t *testing.T) {
	pf := &Promptfile{Version: 1, Prompts: map[string]Source{}}
	err := pf.AddEntry("coder", "alice/coder@1")
	if err != nil {
		t.Fatalf("AddEntry: %v", err)
	}
	if _, ok := pf.Prompts["coder"]; !ok {
		t.Error("entry not added")
	}
}

func TestAddEntryDuplicate(t *testing.T) {
	pf := &Promptfile{Version: 1, Prompts: map[string]Source{
		"coder": {Kind: SourceRegistry, Ref: "alice/coder@1"},
	}}
	err := pf.AddEntry("coder", "alice/coder@2")
	if err == nil {
		t.Fatal("expected error for duplicate")
	}
}

func TestRemoveEntry(t *testing.T) {
	pf := &Promptfile{Version: 1, Prompts: map[string]Source{
		"coder": {Kind: SourceRegistry, Ref: "alice/coder@1"},
	}}
	err := pf.RemoveEntry("coder")
	if err != nil {
		t.Fatalf("RemoveEntry: %v", err)
	}
	if len(pf.Prompts) != 0 {
		t.Error("entry not removed")
	}
}

func TestRemoveEntryNotFound(t *testing.T) {
	pf := &Promptfile{Version: 1, Prompts: map[string]Source{}}
	err := pf.RemoveEntry("nonexistent")
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestRoundTrip(t *testing.T) {
	original := &Promptfile{
		Version: 1,
		Prompts: map[string]Source{
			"alpha": {Kind: SourceRegistry, Ref: "alice/alpha@1"},
			"beta":  {Kind: SourceRegistry, Ref: "bob/beta@2.1.0"},
		},
	}

	data, err := original.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	parsed, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse round-trip: %v", err)
	}

	if len(parsed.Prompts) != len(original.Prompts) {
		t.Fatalf("prompt count: got %d, want %d", len(parsed.Prompts), len(original.Prompts))
	}
	for name, src := range original.Prompts {
		got, ok := parsed.Prompts[name]
		if !ok {
			t.Errorf("missing prompt %q after round-trip", name)
			continue
		}
		if got.Ref != src.Ref {
			t.Errorf("prompt %q: got ref %q, want %q", name, got.Ref, src.Ref)
		}
	}
}

func TestBytesOutput(t *testing.T) {
	pf := &Promptfile{
		Version: 1,
		Prompts: map[string]Source{
			"coder": {Kind: SourceRegistry, Ref: "alice/coder@1"},
		},
	}
	data, err := pf.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "version = 1") {
		t.Error("missing version header")
	}
	if !strings.Contains(out, "[prompts]") {
		t.Error("missing [prompts] section")
	}
	if !strings.Contains(out, `coder = "alice/coder@1"`) {
		t.Errorf("unexpected output: %s", out)
	}
}
