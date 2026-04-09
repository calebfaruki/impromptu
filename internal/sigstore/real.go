package sigstore

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultRekorURL = "https://rekor.sigstore.dev"
	StagingRekorURL = "https://rekor.sigstage.dev"
)

type rekorClient struct {
	baseURL    string
	httpClient *http.Client
}

func newRekorClient(baseURL string) rekorClient {
	return rekorClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Rekor API response types (unexported)

type rekorEntryBody struct {
	Body           string `json:"body"`
	IntegratedTime int64  `json:"integratedTime"`
	LogIndex       int64  `json:"logIndex"`
	LogID          string `json:"logID"`
}

type hashedRekordBody struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Spec       struct {
		Data struct {
			Hash struct {
				Algorithm string `json:"algorithm"`
				Value     string `json:"value"`
			} `json:"hash"`
		} `json:"data"`
		Signature struct {
			Content   string `json:"content"`
			PublicKey struct {
				Content string `json:"content"`
			} `json:"publicKey"`
		} `json:"signature"`
	} `json:"spec"`
}

// fetchAndParseEntry does GET on the given URL and parses the Rekor entry response.
func (c *rekorClient) fetchAndParseEntry(ctx context.Context, url string) (*RekorEntry, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying rekor: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rekor returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var entries map[string]rekorEntryBody
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("parsing rekor response: %w", err)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("rekor entry not found")
	}

	// Take the first (and typically only) entry
	var entry rekorEntryBody
	for _, e := range entries {
		entry = e
		break
	}

	return parseRekorEntry(entry)
}

func parseRekorEntry(entry rekorEntryBody) (*RekorEntry, error) {
	decoded, err := base64.StdEncoding.DecodeString(entry.Body)
	if err != nil {
		return nil, fmt.Errorf("decoding entry body: %w", err)
	}

	var rekord hashedRekordBody
	if err := json.Unmarshal(decoded, &rekord); err != nil {
		return nil, fmt.Errorf("parsing hashedrekord: %w", err)
	}

	if rekord.Kind != "hashedrekord" {
		return nil, fmt.Errorf("unsupported entry kind %q (expected hashedrekord)", rekord.Kind)
	}

	if rekord.Spec.Data.Hash.Algorithm == "" || rekord.Spec.Data.Hash.Value == "" {
		return nil, fmt.Errorf("missing hash algorithm or value in entry")
	}

	digest := rekord.Spec.Data.Hash.Algorithm + ":" + rekord.Spec.Data.Hash.Value

	identity, err := extractSignerIdentity(rekord.Spec.Signature.PublicKey.Content)
	if err != nil {
		return nil, fmt.Errorf("extracting signer identity: %w", err)
	}

	return &RekorEntry{
		LogIndex:       entry.LogIndex,
		Digest:         digest,
		SignerIdentity: identity,
	}, nil
}

func extractSignerIdentity(certContentBase64 string) (string, error) {
	if certContentBase64 == "" {
		return "", fmt.Errorf("empty certificate")
	}

	certPEM, err := base64.StdEncoding.DecodeString(certContentBase64)
	if err != nil {
		return "", fmt.Errorf("decoding certificate base64: %w", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", fmt.Errorf("no PEM block found in certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parsing certificate: %w", err)
	}

	if len(cert.EmailAddresses) > 0 {
		return cert.EmailAddresses[0], nil
	}

	if len(cert.URIs) > 0 {
		return cert.URIs[0].String(), nil
	}

	return "", fmt.Errorf("certificate has no email or URI SAN")
}

// RekorVerifier verifies Rekor transparency log entries by log index.
type RekorVerifier struct {
	client rekorClient
}

func NewRekorVerifier(baseURL string) *RekorVerifier {
	return &RekorVerifier{client: newRekorClient(baseURL)}
}

func (v *RekorVerifier) Verify(ctx context.Context, logIndex int64, expectedDigest string) (*RekorEntry, error) {
	url := fmt.Sprintf("%s/api/v1/log/entries?logIndex=%d", v.client.baseURL, logIndex)
	entry, err := v.client.fetchAndParseEntry(ctx, url)
	if err != nil {
		return nil, err
	}

	if entry.Digest != expectedDigest {
		return nil, fmt.Errorf("digest mismatch: rekor has %q, expected %q", entry.Digest, expectedDigest)
	}

	return entry, nil
}

