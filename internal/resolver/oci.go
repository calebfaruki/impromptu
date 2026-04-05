package resolver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/calebfaruki/impromptu/internal/contentcheck"
	"github.com/calebfaruki/impromptu/internal/lockfile"
	internaloci "github.com/calebfaruki/impromptu/internal/oci"
	"github.com/calebfaruki/impromptu/internal/promptfile"
)

// OCIResult holds the result of resolving an OCI dependency.
type OCIResult struct {
	Entry      lockfile.LockfileEntry
	Dir        string
	CleanupDir string
	Warnings   []string
}

// OCIResolver pulls prompt artifacts from OCI registries.
type OCIResolver struct{}

// Resolve pulls an OCI image, extracts its layer, verifies the digest,
// runs content checks, and returns the result.
func (o *OCIResolver) Resolve(ctx context.Context, src promptfile.Source, force bool) (*OCIResult, error) {
	if src.Kind != promptfile.SourceOCI {
		return nil, fmt.Errorf("oci resolver: expected oci source, got %s", src.Kind)
	}

	// Build reference
	refStr := src.OCI
	if src.OCITag != "" {
		refStr += ":" + src.OCITag
	} else if src.Digest != "" {
		refStr += "@" + src.Digest
	} else {
		return nil, fmt.Errorf("oci source must have tag or digest")
	}

	ref, err := name.ParseReference(refStr)
	if err != nil {
		return nil, fmt.Errorf("parsing OCI reference %q: %w", refStr, err)
	}

	img, err := remote.Image(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, fmt.Errorf("pulling %s: %w", refStr, err)
	}

	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("reading layers from %s: %w", refStr, err)
	}
	if len(layers) == 0 {
		return nil, fmt.Errorf("image %s has no layers", refStr)
	}

	// Read first layer uncompressed (oci.Unpackage expects plain tar)
	rc, err := layers[0].Uncompressed()
	if err != nil {
		return nil, fmt.Errorf("reading layer from %s: %w", refStr, err)
	}
	layerBytes, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil, fmt.Errorf("reading layer bytes from %s: %w", refStr, err)
	}

	// Digest verification
	computed := internaloci.ComputeDigest(layerBytes)

	result := &OCIResult{
		Entry: lockfile.LockfileEntry{
			Source: promptfile.SourceOCI,
			OCI:    src.OCI,
			Tag:    src.OCITag,
			Digest: computed.String(),
		},
	}

	// If user pinned a digest, verify it matches
	if src.Digest != "" && src.Digest != computed.String() {
		return nil, fmt.Errorf("digest mismatch: expected %s, got %s", src.Digest, computed)
	}

	// Extract to temp dir
	tmpDir, err := os.MkdirTemp("", "impromptu-oci-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}

	if err := internaloci.Unpackage(bytes.NewReader(layerBytes), tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("extracting layer from %s: %w", refStr, err)
	}

	// Content checks
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

	// Unsigned check (OCI has no Sigstore bundle)
	if !force {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("%s is unsigned: OCI artifacts have no signature bundle", refStr)
	}
	result.Warnings = append(result.Warnings, "OCI artifact is unsigned")

	result.Dir = tmpDir
	result.CleanupDir = tmpDir
	return result, nil
}
