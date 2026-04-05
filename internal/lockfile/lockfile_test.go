package lockfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/calebfaruki/impromptu/internal/promptfile"
)

// --- Parse tests ---

func TestParseLockfileRegistry(t *testing.T) {
	data := []byte(`version = 1

[[prompt]]
name = "coder"
source = "registry"
ref = "alice/coder@1"
digest = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
signer = "github.com/alice"
`)
	lf, err := ParseLockfile(data)
	if err != nil {
		t.Fatalf("ParseLockfile: %v", err)
	}
	e, ok := lf.Entries["coder"]
	if !ok {
		t.Fatal("missing entry 'coder'")
	}
	if e.Source != promptfile.SourceRegistry {
		t.Errorf("source: got %q", e.Source)
	}
	if e.Digest == "" {
		t.Error("expected non-empty digest")
	}
	if e.Signer != "github.com/alice" {
		t.Errorf("signer: got %q", e.Signer)
	}
}

func TestParseLockfileGit(t *testing.T) {
	data := []byte(`version = 1

[[prompt]]
name = "internal"
source = "git"
git = "https://github.com/org/repo"
tag = "v1"
commit = "abc123"
`)
	lf, err := ParseLockfile(data)
	if err != nil {
		t.Fatalf("ParseLockfile: %v", err)
	}
	e := lf.Entries["internal"]
	if e.Source != promptfile.SourceGit {
		t.Errorf("source: got %q", e.Source)
	}
	if e.Git != "https://github.com/org/repo" {
		t.Errorf("git: got %q", e.Git)
	}
	if e.Commit != "abc123" {
		t.Errorf("commit: got %q", e.Commit)
	}
	if e.Digest != "" {
		t.Errorf("expected empty digest for git, got %q", e.Digest)
	}
}

func TestParseLockfileOCI(t *testing.T) {
	data := []byte(`version = 1

[[prompt]]
name = "img"
source = "oci"
oci = "ghcr.io/org/prompt"
tag = "v1"
digest = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
`)
	lf, err := ParseLockfile(data)
	if err != nil {
		t.Fatalf("ParseLockfile: %v", err)
	}
	e := lf.Entries["img"]
	if e.Source != promptfile.SourceOCI {
		t.Errorf("source: got %q", e.Source)
	}
	if e.Signer != "" {
		t.Errorf("expected empty signer for OCI, got %q", e.Signer)
	}
}

func TestParseLockfileMissingVersion(t *testing.T) {
	data := []byte(`[[prompt]]
name = "coder"
source = "registry"
`)
	_, err := ParseLockfile(data)
	if err == nil {
		t.Fatal("expected error for missing version")
	}
}

func TestLockfileRoundTrip(t *testing.T) {
	original := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"coder": {
				Name:   "coder",
				Source: promptfile.SourceRegistry,
				Ref:    "alice/coder@1",
				Digest: "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
				Signer: "github.com/alice",
			},
		},
	}

	data, err := original.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	parsed, err := ParseLockfile(data)
	if err != nil {
		t.Fatalf("ParseLockfile round-trip: %v", err)
	}

	if len(parsed.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(parsed.Entries))
	}
	e := parsed.Entries["coder"]
	if e.Ref != "alice/coder@1" {
		t.Errorf("ref: got %q", e.Ref)
	}
	if e.Digest != original.Entries["coder"].Digest {
		t.Errorf("digest mismatch after round-trip")
	}
}

// --- Diff tests ---

func TestDiffAdded(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"coder": {Kind: promptfile.SourceRegistry, Ref: "alice/coder@1"},
		},
	}
	lf := &Lockfile{Version: 1, Entries: map[string]LockfileEntry{}}

	result := Diff(pf, lf)
	if len(result.Added) != 1 || result.Added[0] != "coder" {
		t.Errorf("expected [coder] added, got %v", result.Added)
	}
}

func TestDiffRemoved(t *testing.T) {
	pf := &promptfile.Promptfile{Version: 1, Prompts: map[string]promptfile.Source{}}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"old": {Name: "old", Source: promptfile.SourceRegistry, Ref: "alice/old@1"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Removed) != 1 || result.Removed[0] != "old" {
		t.Errorf("expected [old] removed, got %v", result.Removed)
	}
}

func TestDiffUnchanged(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"coder": {Kind: promptfile.SourceRegistry, Ref: "alice/coder@1"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"coder": {Name: "coder", Source: promptfile.SourceRegistry, Ref: "alice/coder@1"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Unchanged) != 1 || result.Unchanged[0] != "coder" {
		t.Errorf("expected [coder] unchanged, got %v", result.Unchanged)
	}
	if len(result.Added) != 0 {
		t.Errorf("expected no added, got %v", result.Added)
	}
}

func TestDiffVersionChange(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"coder": {Kind: promptfile.SourceRegistry, Ref: "alice/coder@2"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"coder": {Name: "coder", Source: promptfile.SourceRegistry, Ref: "alice/coder@1"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Added) != 1 || result.Added[0] != "coder" {
		t.Errorf("version change should be added for re-resolve, got added=%v unchanged=%v", result.Added, result.Unchanged)
	}
}

// --- Verify tests ---

func TestVerifyDigestMatch(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "valid", "simple")
	digest, err := ComputeDirectoryDigest(dir)
	if err != nil {
		t.Fatalf("ComputeDirectoryDigest: %v", err)
	}

	err = VerifyDigest(dir, digest.String())
	if err != nil {
		t.Errorf("expected match, got: %v", err)
	}
}

func TestVerifyDigestMismatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Original"), 0644)

	digest, err := ComputeDirectoryDigest(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Modify file
	os.WriteFile(filepath.Join(dir, "01-context.md"), []byte("# Modified"), 0644)

	err = VerifyDigest(dir, digest.String())
	if err == nil {
		t.Error("expected mismatch error, got nil")
	}
}

func TestVerifyDigestMissingDir(t *testing.T) {
	err := VerifyDigest("/nonexistent/dir", "sha256:abc")
	if err == nil {
		t.Error("expected error for missing dir, got nil")
	}
}

func TestVerifyDigestEmpty(t *testing.T) {
	err := VerifyDigest("/any/path", "")
	if err != nil {
		t.Errorf("empty digest should skip verification, got: %v", err)
	}
}

// --- Diff tests for non-registry sources ---

func TestDiffGitUnchanged(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"internal": {Kind: promptfile.SourceGit, Git: "https://github.com/org/repo", Tag: "v1"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"internal": {Name: "internal", Source: promptfile.SourceGit, Git: "https://github.com/org/repo", Tag: "v1", Commit: "abc123"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Unchanged) != 1 {
		t.Errorf("expected unchanged, got added=%v removed=%v unchanged=%v", result.Added, result.Removed, result.Unchanged)
	}
}

func TestDiffGitTagChange(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"internal": {Kind: promptfile.SourceGit, Git: "https://github.com/org/repo", Tag: "v2"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"internal": {Name: "internal", Source: promptfile.SourceGit, Git: "https://github.com/org/repo", Tag: "v1"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Added) != 1 {
		t.Errorf("tag change should trigger re-resolve, got added=%v unchanged=%v", result.Added, result.Unchanged)
	}
}

func TestDiffGitURLChange(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"internal": {Kind: promptfile.SourceGit, Git: "https://github.com/org/new-repo", Tag: "v1"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"internal": {Name: "internal", Source: promptfile.SourceGit, Git: "https://github.com/org/old-repo", Tag: "v1"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Added) != 1 {
		t.Errorf("git URL change should trigger re-resolve, got added=%v", result.Added)
	}
}

func TestDiffGitPathChange(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"sub": {Kind: promptfile.SourceGit, Git: "https://github.com/org/repo", Tag: "v1", Path: "new-subdir"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"sub": {Name: "sub", Source: promptfile.SourceGit, Git: "https://github.com/org/repo", Tag: "v1", Path: "old-subdir"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Added) != 1 {
		t.Errorf("path change should trigger re-resolve, got added=%v", result.Added)
	}
}

func TestDiffOCIUnchanged(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"img": {Kind: promptfile.SourceOCI, OCI: "ghcr.io/org/prompt", OCITag: "v1"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"img": {Name: "img", Source: promptfile.SourceOCI, OCI: "ghcr.io/org/prompt", Tag: "v1"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Unchanged) != 1 {
		t.Errorf("expected unchanged, got added=%v unchanged=%v", result.Added, result.Unchanged)
	}
}

func TestDiffOCITagChange(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"img": {Kind: promptfile.SourceOCI, OCI: "ghcr.io/org/prompt", OCITag: "v2"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"img": {Name: "img", Source: promptfile.SourceOCI, OCI: "ghcr.io/org/prompt", Tag: "v1"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Added) != 1 {
		t.Errorf("OCI tag change should trigger re-resolve, got added=%v", result.Added)
	}
}

func TestDiffOCIRegistryChange(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"img": {Kind: promptfile.SourceOCI, OCI: "docker.io/org/prompt", OCITag: "v1"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"img": {Name: "img", Source: promptfile.SourceOCI, OCI: "ghcr.io/org/prompt", Tag: "v1"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Added) != 1 {
		t.Errorf("OCI registry change should trigger re-resolve, got added=%v", result.Added)
	}
}

func TestDiffPrivateUnchanged(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"corp": {Kind: promptfile.SourcePrivate, Registry: "https://internal.co", Ref: "team/deploy@latest"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"corp": {Name: "corp", Source: promptfile.SourcePrivate, Registry: "https://internal.co", Ref: "team/deploy@latest"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Unchanged) != 1 {
		t.Errorf("expected unchanged, got added=%v unchanged=%v", result.Added, result.Unchanged)
	}
}

func TestDiffPrivateRefChange(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"corp": {Kind: promptfile.SourcePrivate, Registry: "https://internal.co", Ref: "team/deploy@2"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"corp": {Name: "corp", Source: promptfile.SourcePrivate, Registry: "https://internal.co", Ref: "team/deploy@1"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Added) != 1 {
		t.Errorf("private ref change should trigger re-resolve, got added=%v", result.Added)
	}
}

func TestDiffGitPinnedCommitUnchanged(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"pinned": {Kind: promptfile.SourceGit, Git: "https://github.com/org/repo", Commit: "abc123"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"pinned": {Name: "pinned", Source: promptfile.SourceGit, Git: "https://github.com/org/repo", Commit: "abc123"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Unchanged) != 1 {
		t.Errorf("expected unchanged, got added=%v unchanged=%v", result.Added, result.Unchanged)
	}
}

func TestDiffGitPinnedCommitChanged(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"pinned": {Kind: promptfile.SourceGit, Git: "https://github.com/org/repo", Commit: "newsha"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"pinned": {Name: "pinned", Source: promptfile.SourceGit, Git: "https://github.com/org/repo", Commit: "oldsha"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Added) != 1 {
		t.Errorf("pinned commit change should trigger re-resolve, got added=%v", result.Added)
	}
}

func TestDiffOCIPinnedDigestUnchanged(t *testing.T) {
	digest := "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"pinned": {Kind: promptfile.SourceOCI, OCI: "ghcr.io/org/prompt", Digest: digest},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"pinned": {Name: "pinned", Source: promptfile.SourceOCI, OCI: "ghcr.io/org/prompt", Digest: digest},
		},
	}

	result := Diff(pf, lf)
	if len(result.Unchanged) != 1 {
		t.Errorf("expected unchanged, got added=%v unchanged=%v", result.Added, result.Unchanged)
	}
}

func TestDiffOCIPinnedDigestChanged(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"pinned": {Kind: promptfile.SourceOCI, OCI: "ghcr.io/org/prompt", Digest: "sha256:1111111111111111111111111111111111111111111111111111111111111111"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"pinned": {Name: "pinned", Source: promptfile.SourceOCI, OCI: "ghcr.io/org/prompt", Digest: "sha256:2222222222222222222222222222222222222222222222222222222222222222"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Added) != 1 {
		t.Errorf("pinned digest change should trigger re-resolve, got added=%v", result.Added)
	}
}

func TestDiffSourceKindChange(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"coder": {Kind: promptfile.SourceGit, Git: "https://github.com/alice/coder", Tag: "v1"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"coder": {Name: "coder", Source: promptfile.SourceRegistry, Ref: "alice/coder@1"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Added) != 1 {
		t.Errorf("source kind change should trigger re-resolve, got added=%v", result.Added)
	}
}

// --- Round-trip tests for non-registry entries ---

func TestLockfileRoundTripGit(t *testing.T) {
	original := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"internal": {
				Name:   "internal",
				Source: promptfile.SourceGit,
				Git:    "https://github.com/org/repo",
				Tag:    "v1",
				Commit: "abc123def456",
				Path:   "subdir",
			},
		},
	}

	data, err := original.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	parsed, err := ParseLockfile(data)
	if err != nil {
		t.Fatalf("ParseLockfile: %v", err)
	}

	e := parsed.Entries["internal"]
	if e.Source != promptfile.SourceGit {
		t.Errorf("source: got %q", e.Source)
	}
	if e.Git != "https://github.com/org/repo" {
		t.Errorf("git: got %q", e.Git)
	}
	if e.Tag != "v1" {
		t.Errorf("tag: got %q", e.Tag)
	}
	if e.Commit != "abc123def456" {
		t.Errorf("commit: got %q", e.Commit)
	}
	if e.Path != "subdir" {
		t.Errorf("path: got %q", e.Path)
	}
}

func TestLockfileRoundTripOCI(t *testing.T) {
	original := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"img": {
				Name:   "img",
				Source: promptfile.SourceOCI,
				OCI:    "ghcr.io/org/prompt",
				Tag:    "v2",
				Digest: "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
			},
		},
	}

	data, err := original.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	parsed, err := ParseLockfile(data)
	if err != nil {
		t.Fatalf("ParseLockfile: %v", err)
	}

	e := parsed.Entries["img"]
	if e.Source != promptfile.SourceOCI {
		t.Errorf("source: got %q", e.Source)
	}
	if e.OCI != "ghcr.io/org/prompt" {
		t.Errorf("oci: got %q", e.OCI)
	}
	if e.Tag != "v2" {
		t.Errorf("tag: got %q", e.Tag)
	}
	if e.Digest != original.Entries["img"].Digest {
		t.Errorf("digest: got %q", e.Digest)
	}
}

func TestLockfileRoundTripPrivate(t *testing.T) {
	original := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"corp": {
				Name:     "corp",
				Source:   promptfile.SourcePrivate,
				Registry: "https://internal.co",
				Ref:      "team/deploy@latest",
				Digest:   "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
				Signer:   "github.com/teamlead",
			},
		},
	}

	data, err := original.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	parsed, err := ParseLockfile(data)
	if err != nil {
		t.Fatalf("ParseLockfile: %v", err)
	}

	e := parsed.Entries["corp"]
	if e.Source != promptfile.SourcePrivate {
		t.Errorf("source: got %q", e.Source)
	}
	if e.Registry != "https://internal.co" {
		t.Errorf("registry: got %q", e.Registry)
	}
	if e.Ref != "team/deploy@latest" {
		t.Errorf("ref: got %q", e.Ref)
	}
	if e.Signer != "github.com/teamlead" {
		t.Errorf("signer: got %q", e.Signer)
	}
}

func TestComputeDirectoryDigestDeterministic(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "valid", "simple")
	d1, err := ComputeDirectoryDigest(dir)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := ComputeDirectoryDigest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d2 {
		t.Errorf("not deterministic: %s vs %s", d1, d2)
	}
}
