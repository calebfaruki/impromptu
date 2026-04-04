package sigstore

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// fakeBundle is the JSON structure produced by FakeSigner.
type fakeBundle struct {
	Digest   string `json:"digest"`
	Identity string `json:"identity"`
	Time     string `json:"timestamp"`
	Index    int64  `json:"index"`
}

// FakeSigner produces deterministic fake signature bundles for testing.
type FakeSigner struct {
	Err       error
	mu        sync.Mutex
	nextIndex int64
}

func (f *FakeSigner) Sign(_ context.Context, digest string, identity string) (SignatureBundle, error) {
	if f.Err != nil {
		return SignatureBundle{}, f.Err
	}

	f.mu.Lock()
	idx := f.nextIndex
	f.nextIndex++
	f.mu.Unlock()

	b := fakeBundle{
		Digest:   digest,
		Identity: identity,
		Time:     time.Now().UTC().Format(time.RFC3339),
		Index:    idx,
	}
	data, err := json.Marshal(b)
	if err != nil {
		return SignatureBundle{}, fmt.Errorf("marshaling fake bundle: %w", err)
	}

	return SignatureBundle{
		BundleJSON:     data,
		RekorLogIndex:  idx,
		SignerIdentity: identity,
	}, nil
}

// FakeVerifier verifies fake signature bundles by parsing and comparing fields.
type FakeVerifier struct {
	Err error
}

func (f *FakeVerifier) Verify(_ context.Context, bundleJSON []byte, expectedDigest string, expectedIdentity string) error {
	if f.Err != nil {
		return f.Err
	}
	if len(bundleJSON) == 0 {
		return fmt.Errorf("verifying signature: empty bundle")
	}

	var b fakeBundle
	if err := json.Unmarshal(bundleJSON, &b); err != nil {
		return fmt.Errorf("verifying signature: parsing bundle: %w", err)
	}
	if b.Digest != expectedDigest {
		return fmt.Errorf("verifying signature: digest mismatch: bundle has %q, expected %q", b.Digest, expectedDigest)
	}
	if b.Identity != expectedIdentity {
		return fmt.Errorf("verifying signature: identity mismatch: bundle has %q, expected %q", b.Identity, expectedIdentity)
	}
	return nil
}
