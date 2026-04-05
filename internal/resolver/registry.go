package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/calebfaruki/impromptu/internal/lockfile"
	"github.com/calebfaruki/impromptu/internal/oci"
	"github.com/calebfaruki/impromptu/internal/promptfile"
	"github.com/calebfaruki/impromptu/internal/sigstore"
)

const cooldownDuration = 72 * time.Hour

// VersionInfo represents version metadata from the registry API.
type VersionInfo struct {
	Version         string `json:"version"`
	Digest          string `json:"digest"`
	SignatureBundle string `json:"signature_bundle"`
	RekorLogIndex   int64  `json:"rekor_log_index"`
	CreatedAt       string `json:"created_at"`
}

// ResolveResult holds the resolved artifact and any warnings from forced bypasses.
type ResolveResult struct {
	Entry    lockfile.LockfileEntry
	Blob     []byte
	Warnings []string
}

// RegistryClient resolves prompts from an Impromptu registry.
type RegistryClient struct {
	baseURL  string
	client   *http.Client
	verifier sigstore.Verifier
}

// NewRegistryClient creates a resolver for the given registry base URL.
func NewRegistryClient(baseURL string, verifier sigstore.Verifier) *RegistryClient {
	return &RegistryClient{
		baseURL:  strings.TrimRight(baseURL, "/"),
		client:   &http.Client{Timeout: 30 * time.Second},
		verifier: verifier,
	}
}

// Resolve fetches a prompt from the registry, resolves the version, verifies
// digest and signature, checks cooldown, and returns the result.
func (r *RegistryClient) Resolve(ctx context.Context, ref string, force bool) (*ResolveResult, error) {
	author, name, versionSpec, err := promptfile.ParseRef(ref)
	if err != nil {
		return nil, err
	}

	versions, err := r.fetchVersions(ctx, author, name)
	if err != nil {
		return nil, err
	}

	matched, err := matchVersion(versions, versionSpec)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", ref, err)
	}

	blob, err := r.fetchBlob(ctx, matched.Digest)
	if err != nil {
		return nil, err
	}

	// Digest verification (unconditional -- data corruption is never bypassed)
	computed := oci.ComputeDigest(blob)
	if computed.String() != matched.Digest {
		return nil, fmt.Errorf("digest mismatch: expected %s, got %s", matched.Digest, computed)
	}

	result := &ResolveResult{
		Entry: lockfile.LockfileEntry{
			Source: promptfile.SourceRegistry,
			Ref:    ref,
			Digest: matched.Digest,
		},
		Blob: blob,
	}

	// Signature verification (unsigned artifacts fail by default)
	identity := "github.com/" + author
	if matched.SignatureBundle == "" {
		if !force {
			return nil, fmt.Errorf("%s is unsigned: no signature bundle", ref)
		}
		result.Warnings = append(result.Warnings, "artifact is unsigned")
	} else {
		err := r.verifier.Verify(ctx, []byte(matched.SignatureBundle), matched.Digest, identity)
		if err != nil {
			if !force {
				return nil, fmt.Errorf("signature verification failed for %s: %w", ref, err)
			}
			result.Warnings = append(result.Warnings, fmt.Sprintf("signature verification failed: %v", err))
		}
		result.Entry.Signer = identity
	}

	// Cooldown check
	createdAt, err := time.Parse("2006-01-02T15:04:05Z", matched.CreatedAt)
	if err == nil && time.Since(createdAt) < cooldownDuration {
		age := time.Since(createdAt).Truncate(time.Hour)
		if !force {
			return nil, fmt.Errorf("%s published %s ago, use --force to bypass 72-hour cooldown", ref, age)
		}
		result.Warnings = append(result.Warnings, fmt.Sprintf("artifact published %s ago (< 72 hours)", age))
	}

	return result, nil
}

func (r *RegistryClient) fetchVersions(ctx context.Context, author, name string) ([]VersionInfo, error) {
	url := fmt.Sprintf("%s/api/v1/prompts/%s/%s/versions", r.baseURL, author, name)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching versions for %s/%s: %w", author, name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("prompt not found: %s/%s", author, name)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned %d for %s/%s", resp.StatusCode, author, name)
	}

	var data struct {
		Versions []VersionInfo `json:"versions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decoding versions: %w", err)
	}
	return data.Versions, nil
}

func (r *RegistryClient) fetchBlob(ctx context.Context, digest string) ([]byte, error) {
	url := fmt.Sprintf("%s/api/v1/blobs/%s", r.baseURL, digest)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching blob %s: %w", digest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("blob %s not found (status %d)", digest, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading blob %s: %w", digest, err)
	}
	return data, nil
}

func matchVersion(versions []VersionInfo, spec string) (VersionInfo, error) {
	if len(versions) == 0 {
		return VersionInfo{}, fmt.Errorf("no versions available")
	}

	if spec == "latest" {
		return versions[0], nil
	}

	// Digest pin: sha256:...
	if strings.HasPrefix(spec, "sha256:") {
		for _, v := range versions {
			if v.Digest == spec {
				return v, nil
			}
		}
		return VersionInfo{}, fmt.Errorf("no version with digest %s", spec)
	}

	// Exact semver: N.N.N
	if strings.Count(spec, ".") == 2 {
		for _, v := range versions {
			if v.Version == spec {
				return v, nil
			}
		}
		return VersionInfo{}, fmt.Errorf("version %s not found", spec)
	}

	// Major only: N -> latest N.x.x
	major, err := strconv.Atoi(spec)
	if err != nil {
		return VersionInfo{}, fmt.Errorf("unsupported version spec %q", spec)
	}
	for _, v := range versions {
		parts := strings.SplitN(v.Version, ".", 2)
		if len(parts) > 0 {
			m, err := strconv.Atoi(parts[0])
			if err == nil && m == major {
				return v, nil
			}
		}
	}
	return VersionInfo{}, fmt.Errorf("no version matching major %d", major)
}
