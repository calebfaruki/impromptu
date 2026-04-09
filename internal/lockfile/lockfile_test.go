package lockfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/calebfaruki/impromptu/internal/promptfile"
)

// --- Parse tests ---

func TestParseLockfileClone(t *testing.T) {
	data := []byte(`version = 1

[[prompt]]
name = "coder"
source = "git"
git = "https://github.com/alice/coder"
ref = "v1"
ref_type = "tag"
commit = "abc123"
digest = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
signer = "alice@github.com"
`)
	lf, err := ParseLockfile(data)
	if err != nil {
		t.Fatalf("ParseLockfile: %v", err)
	}
	e := lf.Entries["coder"]
	if e.Source != promptfile.SourceGit {
		t.Errorf("source: got %q", e.Source)
	}
	if e.Ref != "v1" {
		t.Errorf("ref: got %q", e.Ref)
	}
	if e.RefType != "tag" {
		t.Errorf("ref_type: got %q", e.RefType)
	}
	if e.Commit != "abc123" {
		t.Errorf("commit: got %q", e.Commit)
	}
	if e.Signer != "alice@github.com" {
		t.Errorf("signer: got %q", e.Signer)
	}
}

func TestParseLockfileRelease(t *testing.T) {
	data := []byte(`version = 1

[[prompt]]
name = "deploy"
source = "release"
git = "https://github.com/alice/deploy"
release = "v2.0.0"
asset = "deploy.tar.gz"
digest = "sha256:abc123"
signer = "alice@github.com"
`)
	lf, err := ParseLockfile(data)
	if err != nil {
		t.Fatalf("ParseLockfile: %v", err)
	}
	e := lf.Entries["deploy"]
	if e.Source != promptfile.SourceRelease {
		t.Errorf("source: got %q", e.Source)
	}
	if e.Release != "v2.0.0" {
		t.Errorf("release: got %q", e.Release)
	}
	if e.Asset != "deploy.tar.gz" {
		t.Errorf("asset: got %q", e.Asset)
	}
}

// --- Migration tests ---

func TestMigrateOldTagEntry(t *testing.T) {
	data := []byte(`version = 1

[[prompt]]
name = "coder"
source = "git"
git = "https://github.com/alice/coder"
tag = "v1"
commit = "abc123"
`)
	lf, err := ParseLockfile(data)
	if err != nil {
		t.Fatalf("ParseLockfile: %v", err)
	}
	e := lf.Entries["coder"]
	if e.Ref != "v1" {
		t.Errorf("ref: got %q, want v1", e.Ref)
	}
	if e.RefType != "tag" {
		t.Errorf("ref_type: got %q, want tag", e.RefType)
	}
	if e.Commit != "abc123" {
		t.Errorf("commit: got %q", e.Commit)
	}
}

func TestMigrateOldBranchEntry(t *testing.T) {
	data := []byte(`version = 1

[[prompt]]
name = "dev"
source = "git"
git = "https://github.com/alice/coder"
branch = "main"
commit = "def456"
`)
	lf, err := ParseLockfile(data)
	if err != nil {
		t.Fatalf("ParseLockfile: %v", err)
	}
	e := lf.Entries["dev"]
	if e.Ref != "main" {
		t.Errorf("ref: got %q, want main", e.Ref)
	}
	if e.RefType != "branch" {
		t.Errorf("ref_type: got %q, want branch", e.RefType)
	}
}

func TestMigrateOldOCISkipped(t *testing.T) {
	data := []byte(`version = 1

[[prompt]]
name = "img"
source = "oci"
oci = "ghcr.io/org/prompt"
tag = "v1"
`)
	lf, err := ParseLockfile(data)
	if err != nil {
		t.Fatalf("ParseLockfile: %v", err)
	}
	if _, ok := lf.Entries["img"]; ok {
		t.Error("OCI entries should be skipped")
	}
}

func TestParseLockfileMissingVersion(t *testing.T) {
	data := []byte(`[[prompt]]
name = "coder"
source = "git"
`)
	_, err := ParseLockfile(data)
	if err == nil {
		t.Fatal("expected error for missing version")
	}
}

// --- Round-trip tests ---

func TestLockfileRoundTripClone(t *testing.T) {
	original := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"coder": {
				Name:    "coder",
				Source:  promptfile.SourceGit,
				Git:     "https://github.com/alice/coder",
				Ref:     "v1",
				RefType: "tag",
				Commit:  "abc123def456",
				Path:    "subdir",
				Digest:  "sha256:abc",
				Signer:  "alice@github.com",
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

	e := parsed.Entries["coder"]
	if e.Ref != "v1" {
		t.Errorf("ref: got %q", e.Ref)
	}
	if e.RefType != "tag" {
		t.Errorf("ref_type: got %q", e.RefType)
	}
	if e.Commit != "abc123def456" {
		t.Errorf("commit: got %q", e.Commit)
	}
	if e.Path != "subdir" {
		t.Errorf("path: got %q", e.Path)
	}
}

func TestLockfileRoundTripRelease(t *testing.T) {
	original := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"deploy": {
				Name:    "deploy",
				Source:  promptfile.SourceRelease,
				Git:     "https://github.com/alice/deploy",
				Release: "v2.0.0",
				Asset:   "custom.tar.gz",
				Digest:  "sha256:def",
				Signer:  "alice@github.com",
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

	e := parsed.Entries["deploy"]
	if e.Source != promptfile.SourceRelease {
		t.Errorf("source: got %q", e.Source)
	}
	if e.Release != "v2.0.0" {
		t.Errorf("release: got %q", e.Release)
	}
	if e.Asset != "custom.tar.gz" {
		t.Errorf("asset: got %q", e.Asset)
	}
}

// --- Diff tests ---

func TestDiffAdded(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"coder": {Kind: promptfile.SourceGit, Git: "https://github.com/alice/coder", Ref: "v1"},
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
			"old": {Name: "old", Source: promptfile.SourceGit, Git: "https://github.com/alice/old", Ref: "v1"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Removed) != 1 || result.Removed[0] != "old" {
		t.Errorf("expected [old] removed, got %v", result.Removed)
	}
}

func TestDiffUnchangedClone(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"coder": {Kind: promptfile.SourceGit, Git: "https://github.com/alice/coder", Ref: "v1"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"coder": {Name: "coder", Source: promptfile.SourceGit, Git: "https://github.com/alice/coder", Ref: "v1", Commit: "abc123"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Unchanged) != 1 {
		t.Errorf("expected unchanged, got added=%v unchanged=%v", result.Added, result.Unchanged)
	}
}

func TestDiffRefChange(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"coder": {Kind: promptfile.SourceGit, Git: "https://github.com/alice/coder", Ref: "v2"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"coder": {Name: "coder", Source: promptfile.SourceGit, Git: "https://github.com/alice/coder", Ref: "v1"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Added) != 1 {
		t.Errorf("ref change should trigger re-resolve, got added=%v unchanged=%v", result.Added, result.Unchanged)
	}
}

func TestDiffUnchangedRelease(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"deploy": {Kind: promptfile.SourceRelease, Git: "https://github.com/alice/deploy", Release: "v1"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"deploy": {Name: "deploy", Source: promptfile.SourceRelease, Git: "https://github.com/alice/deploy", Release: "v1"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Unchanged) != 1 {
		t.Errorf("expected unchanged, got added=%v unchanged=%v", result.Added, result.Unchanged)
	}
}

func TestDiffReleaseChange(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"deploy": {Kind: promptfile.SourceRelease, Git: "https://github.com/alice/deploy", Release: "v2"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"deploy": {Name: "deploy", Source: promptfile.SourceRelease, Git: "https://github.com/alice/deploy", Release: "v1"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Added) != 1 {
		t.Errorf("release change should trigger re-resolve, got added=%v", result.Added)
	}
}

func TestDiffKindChange(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"coder": {Kind: promptfile.SourceRelease, Git: "https://github.com/alice/coder", Release: "v1"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"coder": {Name: "coder", Source: promptfile.SourceGit, Git: "https://github.com/alice/coder", Ref: "v1"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Added) != 1 {
		t.Errorf("kind change should trigger re-resolve, got added=%v", result.Added)
	}
}

func TestDiffPathChange(t *testing.T) {
	pf := &promptfile.Promptfile{
		Version: 1,
		Prompts: map[string]promptfile.Source{
			"sub": {Kind: promptfile.SourceGit, Git: "https://github.com/alice/repo", Ref: "v1", Path: "new-subdir"},
		},
	}
	lf := &Lockfile{
		Version: 1,
		Entries: map[string]LockfileEntry{
			"sub": {Name: "sub", Source: promptfile.SourceGit, Git: "https://github.com/alice/repo", Ref: "v1", Path: "old-subdir"},
		},
	}

	result := Diff(pf, lf)
	if len(result.Added) != 1 {
		t.Errorf("path change should trigger re-resolve, got added=%v", result.Added)
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
