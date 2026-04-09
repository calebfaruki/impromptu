package resolver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/verify"

	"github.com/calebfaruki/impromptu/internal/authprobe"
	"github.com/calebfaruki/impromptu/internal/contentcheck"
	"github.com/calebfaruki/impromptu/internal/lockfile"
	"github.com/calebfaruki/impromptu/internal/oci"
	"github.com/calebfaruki/impromptu/internal/promptfile"
)

// ReleaseResolver downloads signed release assets from git forges.
type ReleaseResolver struct {
	Progress   io.Writer
	HTTPClient *http.Client
	BaseURL    string // override for testing; if set, replaces the host in asset URLs
}

// ReleaseResult holds the result of resolving a release dependency.
type ReleaseResult struct {
	Entry      lockfile.LockfileEntry
	Blob       []byte
	CleanupDir string
	Warnings   []string
}

func (r *ReleaseResolver) client() *http.Client {
	if r.HTTPClient != nil {
		return r.HTTPClient
	}
	return &http.Client{Timeout: 60 * time.Second}
}

// Resolve downloads a tarball and sigstore bundle from a git forge release,
// verifies the bundle, runs content checks, and returns the result.
func (r *ReleaseResolver) Resolve(ctx context.Context, src promptfile.Source, force bool) (*ReleaseResult, error) {
	if src.Kind != promptfile.SourceRelease {
		return nil, fmt.Errorf("release resolver: expected release source, got %s", src.Kind)
	}

	assetURL, err := buildAssetURL(src)
	if err != nil {
		return nil, err
	}
	if r.BaseURL != "" {
		_, owner, repo := authprobe.ParseSourceURL(src.Git)
		assetName := repo + ".tar.gz"
		if src.Asset != "" {
			assetName = src.Asset
		}
		assetURL = fmt.Sprintf("%s/%s/%s/releases/download/%s/%s", r.BaseURL, owner, repo, src.Release, assetName)
	}
	bundleURL := assetURL + ".sigstore.json"

	r.logProgress("downloading %s\n", assetURL)

	tarball, err := r.download(ctx, assetURL)
	if err != nil {
		return nil, fmt.Errorf("downloading release asset: %w", err)
	}

	bundleBytes, err := r.download(ctx, bundleURL)
	bundleFound := err == nil

	result := &ReleaseResult{
		Entry: lockfile.LockfileEntry{
			Source:  promptfile.SourceRelease,
			Git:     src.Git,
			Release: src.Release,
			Asset:   src.Asset,
		},
	}

	if !bundleFound && !force {
		return nil, fmt.Errorf("sigstore bundle not found at %s; release mode requires signed artifacts (use --force to bypass)", bundleURL)
	}
	if !bundleFound {
		result.Warnings = append(result.Warnings, "unsigned release, no sigstore bundle found")
	}

	if bundleFound {
		bv, err := verifyBundle(bundleBytes, tarball)
		if err != nil {
			if !force {
				return nil, fmt.Errorf("sigstore verification failed: %w", err)
			}
			result.Warnings = append(result.Warnings, fmt.Sprintf("sigstore verification failed (bypassed with --force): %v", err))
		} else {
			result.Entry.Signer = bv.Signer
			result.Entry.RekorLogIndex = bv.RekorLogIndex
		}
	}

	// Extract tarball to temp dir and run content checks
	tmpDir, err := os.MkdirTemp("", "impromptu-release-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}

	if err := oci.UnpackageBytes(tarball, tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("extracting tarball: %w", err)
	}

	violations, err := contentcheck.CheckDirectory(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("content check: %w", err)
	}
	if len(violations) > 0 {
		var msgs []string
		for _, v := range violations {
			msgs = append(msgs, v.Error())
		}
		if !force {
			os.RemoveAll(tmpDir)
			return nil, fmt.Errorf("content check failed:\n%s", strings.Join(msgs, "\n"))
		}
		result.Warnings = append(result.Warnings, "content check violations bypassed with --force")
	}

	result.Entry.Digest = oci.ComputeDigest(tarball).String()
	result.Blob = tarball
	result.CleanupDir = tmpDir
	return result, nil
}

func buildAssetURL(src promptfile.Source) (string, error) {
	host, owner, repo := authprobe.ParseSourceURL(src.Git)

	assetName := repo + ".tar.gz"
	if src.Asset != "" {
		assetName = src.Asset
	}

	switch host {
	case "github.com":
		return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", owner, repo, src.Release, assetName), nil
	case "codeberg.org":
		return fmt.Sprintf("https://codeberg.org/%s/%s/releases/download/%s/%s", owner, repo, src.Release, assetName), nil
	default:
		return "", fmt.Errorf("unsupported host %q for release downloads (supported: github.com, codeberg.org)", host)
	}
}

func (r *ReleaseResolver) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("not found: %s", url)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}

	return io.ReadAll(resp.Body)
}

type bundleVerification struct {
	Signer        string
	RekorLogIndex int64
}

// verifyBundle verifies a sigstore bundle against artifact bytes.
// Returns the signer identity and Rekor log index.
func verifyBundle(bundleJSON, artifact []byte) (*bundleVerification, error) {
	b := &bundle.Bundle{}
	if err := b.UnmarshalJSON(bundleJSON); err != nil {
		return nil, fmt.Errorf("parsing sigstore bundle: %w", err)
	}

	trustedRoot, err := root.FetchTrustedRoot()
	if err != nil {
		return nil, fmt.Errorf("fetching sigstore trusted root: %w", err)
	}

	verifier, err := verify.NewVerifier(trustedRoot,
		verify.WithTransparencyLog(1),
		verify.WithObserverTimestamps(1),
	)
	if err != nil {
		return nil, fmt.Errorf("creating verifier: %w", err)
	}

	policy := verify.NewPolicy(
		verify.WithArtifact(bytes.NewReader(artifact)),
		verify.WithoutIdentitiesUnsafe(),
	)

	result, err := verifier.Verify(b, policy)
	if err != nil {
		return nil, err
	}

	bv := &bundleVerification{}
	if result.Signature != nil && result.Signature.Certificate != nil {
		bv.Signer = result.Signature.Certificate.SubjectAlternativeName
	}

	entries, err := b.TlogEntries()
	if err == nil && len(entries) > 0 {
		bv.RekorLogIndex = entries[0].LogIndex()
	}

	return bv, nil
}

func (r *ReleaseResolver) logProgress(format string, args ...any) {
	if r.Progress != nil {
		fmt.Fprintf(r.Progress, format, args...)
	}
}
