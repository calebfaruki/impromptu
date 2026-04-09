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

// --- Parse: clone mode ---

func TestParseCloneRef(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\ncoder = {git = \"https://github.com/alice/coder\", ref = \"v1\"}\n")
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
	if src.Ref != "v1" {
		t.Errorf("ref: got %q", src.Ref)
	}
}

func TestParseCloneWithPath(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nwriter = {git = \"https://github.com/alice/prompts\", ref = \"v1\", path = \"writer\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if pf.Prompts["writer"].Path != "writer" {
		t.Errorf("path: got %q", pf.Prompts["writer"].Path)
	}
}

func TestParseCloneSSH(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\ninternal = {git = \"git@github.com:alice/coder.git\", ref = \"v2\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if pf.Prompts["internal"].Git != "git@github.com:alice/coder.git" {
		t.Errorf("git: got %q", pf.Prompts["internal"].Git)
	}
}

func TestParseCloneInline(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nclaude = {git = \"https://github.com/alice/claude-md\", ref = \"v1\", inline = true}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !pf.Prompts["claude"].Inline {
		t.Error("expected inline = true")
	}
}

// --- Parse: release mode ---

func TestParseRelease(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\ncoder = {git = \"https://github.com/alice/coder\", release = \"v1\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	src := pf.Prompts["coder"]
	if src.Kind != SourceRelease {
		t.Errorf("kind: got %q", src.Kind)
	}
	if src.Release != "v1" {
		t.Errorf("release: got %q", src.Release)
	}
}

func TestParseReleaseWithAsset(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\ncoder = {git = \"https://github.com/alice/coder\", release = \"v1\", asset = \"custom.tar.gz\"}\n")
	pf, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if pf.Prompts["coder"].Asset != "custom.tar.gz" {
		t.Errorf("asset: got %q", pf.Prompts["coder"].Asset)
	}
}

// --- Parse: rejections ---

func TestRejectRefAndRelease(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nbad = {git = \"https://github.com/alice/coder\", ref = \"v1\", release = \"v1\"}\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for ref + release")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should mention mutually exclusive: %v", err)
	}
}

func TestRejectNeitherRefNorRelease(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nbad = {git = \"https://github.com/alice/coder\"}\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for neither ref nor release")
	}
	if !strings.Contains(err.Error(), "must have ref or release") {
		t.Errorf("error should mention ref or release: %v", err)
	}
}

func TestRejectReleaseWithPath(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nbad = {git = \"https://github.com/alice/coder\", release = \"v1\", path = \"sub\"}\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for release + path")
	}
}

func TestRejectCloneWithAsset(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nbad = {git = \"https://github.com/alice/coder\", ref = \"v1\", asset = \"x.tar.gz\"}\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for ref + asset")
	}
}

func TestRejectUnsupportedHostInReleaseFlags(t *testing.T) {
	_, err := SourceFromFlags("https://gitlab.com/alice/coder", "", "v1", "", "", false)
	if err == nil {
		t.Fatal("expected error for unsupported host in release mode")
	}
	if !strings.Contains(err.Error(), "unsupported git host") {
		t.Errorf("error should mention unsupported host: %v", err)
	}
}

func TestAllowUnsupportedHostInCloneMode(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nbad = {git = \"https://gitlab.com/alice/coder\", ref = \"v1\"}\n")
	_, err := Parse(data)
	if err != nil {
		t.Fatalf("clone mode should allow any host: %v", err)
	}
}

func TestRejectOCISource(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nreviewer = {oci = \"ghcr.io/alice/reviewer\", tag = \"v1\"}\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error for OCI source")
	}
	if !strings.Contains(err.Error(), "OCI sources are not supported") {
		t.Errorf("error should mention OCI not supported: %v", err)
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
	data := []byte("version = 1\n\n[prompts]\nbad = {git = \"https://github.com/alice/coder\", ref = \"v1\", path = \"../escape\"}\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRejectAbsolutePath(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nbad = {git = \"https://github.com/alice/coder\", ref = \"v1\", path = \"/etc/prompts\"}\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRejectBackslashPath(t *testing.T) {
	data := []byte("version = 1\n\n[prompts]\nbad = {git = \"https://github.com/alice/coder\", ref = \"v1\", path = \"sub\\\\dir\"}\n")
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- SourceFromFlags ---

func TestSourceFromFlagsClone(t *testing.T) {
	src, err := SourceFromFlags("https://github.com/alice/coder", "v1", "", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if src.Kind != SourceGit || src.Ref != "v1" {
		t.Errorf("got %+v", src)
	}
}

func TestSourceFromFlagsRelease(t *testing.T) {
	src, err := SourceFromFlags("https://github.com/alice/coder", "", "v1", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if src.Kind != SourceRelease || src.Release != "v1" {
		t.Errorf("got %+v", src)
	}
}

func TestSourceFromFlagsReleaseWithAsset(t *testing.T) {
	src, err := SourceFromFlags("https://github.com/alice/coder", "", "v1", "", "custom.tar.gz", false)
	if err != nil {
		t.Fatal(err)
	}
	if src.Asset != "custom.tar.gz" {
		t.Errorf("asset: got %q", src.Asset)
	}
}

func TestSourceFromFlagsNoGit(t *testing.T) {
	_, err := SourceFromFlags("", "v1", "", "", "", false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSourceFromFlagsInline(t *testing.T) {
	src, err := SourceFromFlags("https://github.com/alice/claude-md", "v1", "", "", "", true)
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

func TestAliasFromRelease(t *testing.T) {
	if a := AliasFromSource(Source{Kind: SourceRelease, Git: "https://github.com/alice/coder"}); a != "coder" {
		t.Errorf("got %q", a)
	}
}

// --- Write ---

func TestAddSourceAndRemove(t *testing.T) {
	pf := &Promptfile{Version: 1, Prompts: map[string]Source{}}
	if err := pf.AddSource("coder", Source{Kind: SourceGit, Git: "https://github.com/alice/coder", Ref: "v1"}); err != nil {
		t.Fatal(err)
	}
	if err := pf.RemoveEntry("coder"); err != nil {
		t.Fatal(err)
	}
	if len(pf.Prompts) != 0 {
		t.Error("not removed")
	}
}

func TestRoundTripClone(t *testing.T) {
	original := &Promptfile{
		Version: 1,
		Prompts: map[string]Source{
			"coder":  {Kind: SourceGit, Git: "https://github.com/alice/coder", Ref: "v1"},
			"writer": {Kind: SourceGit, Git: "https://github.com/alice/writer", Ref: "main", Path: "prompts"},
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
	if parsed.Prompts["coder"].Ref != "v1" {
		t.Error("coder ref mismatch")
	}
	if parsed.Prompts["writer"].Ref != "main" {
		t.Error("writer ref mismatch")
	}
	if parsed.Prompts["writer"].Path != "prompts" {
		t.Error("writer path mismatch")
	}
}

func TestRoundTripRelease(t *testing.T) {
	original := &Promptfile{
		Version: 1,
		Prompts: map[string]Source{
			"deploy": {Kind: SourceRelease, Git: "https://github.com/alice/deploy", Release: "v2.0.0", Asset: "custom.tar.gz"},
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
	src := parsed.Prompts["deploy"]
	if src.Kind != SourceRelease {
		t.Errorf("kind: got %q", src.Kind)
	}
	if src.Release != "v2.0.0" {
		t.Errorf("release: got %q", src.Release)
	}
	if src.Asset != "custom.tar.gz" {
		t.Errorf("asset: got %q", src.Asset)
	}
}

func TestRoundTripInline(t *testing.T) {
	original := &Promptfile{
		Version: 1,
		Prompts: map[string]Source{
			"claude": {Kind: SourceGit, Git: "https://github.com/alice/claude-md", Ref: "v1", Inline: true},
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
