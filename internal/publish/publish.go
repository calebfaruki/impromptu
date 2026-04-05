package publish

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/calebfaruki/impromptu/internal/contentcheck"
	"github.com/calebfaruki/impromptu/internal/oci"
	"github.com/calebfaruki/impromptu/internal/sigstore"
)

// PublishConfig holds the dependencies and parameters for a publish operation.
type PublishConfig struct {
	Dir         string
	Name        string
	Description string
	Version     string
	RegistryURL string
	Token       string
	Signer      sigstore.Signer
	Identity    string
}

// PublishResult reports what was published.
type PublishResult struct {
	Digest  string
	Name    string
	Version string
}

// Publish packages, checks, signs, and pushes a prompt directory to the registry.
func Publish(ctx context.Context, cfg PublishConfig) (*PublishResult, error) {
	files, err := CollectFiles(cfg.Dir)
	if err != nil {
		return nil, err
	}

	// Copy filtered files to temp dir for packaging
	tmpDir, err := os.MkdirTemp("", "impromptu-publish-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		dest := filepath.Join(tmpDir, filepath.Base(path))
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", dest, err)
		}
	}

	// Content check
	violations, err := contentcheck.CheckDirectory(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("content check: %w", err)
	}
	if len(violations) > 0 {
		var msgs []string
		for _, v := range violations {
			msgs = append(msgs, v.Error())
		}
		return nil, fmt.Errorf("content check failed:\n%s", joinLines(msgs))
	}

	// Package
	tarData, err := oci.PackageBytes(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("packaging: %w", err)
	}
	digest := oci.ComputeDigest(tarData)

	// Sign
	bundle, err := cfg.Signer.Sign(ctx, digest.String(), cfg.Identity)
	if err != nil {
		return nil, fmt.Errorf("signing: %w", err)
	}

	// Push to registry
	if err := pushToRegistry(ctx, cfg, tarData, digest.String(), bundle); err != nil {
		return nil, fmt.Errorf("pushing to registry: %w", err)
	}

	return &PublishResult{
		Digest:  digest.String(),
		Name:    cfg.Name,
		Version: cfg.Version,
	}, nil
}

func pushToRegistry(ctx context.Context, cfg PublishConfig, tarData []byte, digest string, bundle sigstore.SignatureBundle) error {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("name", cfg.Name)
	mw.WriteField("description", cfg.Description)
	mw.WriteField("version", cfg.Version)
	mw.WriteField("signature_bundle", string(bundle.BundleJSON))
	mw.WriteField("rekor_log_index", fmt.Sprintf("%d", bundle.RekorLogIndex))

	fw, err := mw.CreateFormFile("archive", "prompt.tar")
	if err != nil {
		return fmt.Errorf("creating form file: %w", err)
	}
	if _, err := io.Copy(fw, bytes.NewReader(tarData)); err != nil {
		return fmt.Errorf("writing archive: %w", err)
	}
	mw.Close()

	url := fmt.Sprintf("%s/api/v1/publish", cfg.RegistryURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, &body)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("posting to registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("registry returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func joinLines(lines []string) string {
	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += l
	}
	return result
}
