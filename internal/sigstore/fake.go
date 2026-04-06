package sigstore

import (
	"context"
	"fmt"
)

// FakeVerifier is a Verifier for testing. Register entries with AddEntry,
// then Verify looks them up by log index and compares digests.
type FakeVerifier struct {
	Err     error
	entries map[int64]*RekorEntry
}

// NewFakeVerifier creates a FakeVerifier with no entries.
func NewFakeVerifier() *FakeVerifier {
	return &FakeVerifier{entries: make(map[int64]*RekorEntry)}
}

// AddEntry registers a Rekor entry for testing.
func (f *FakeVerifier) AddEntry(entry RekorEntry) {
	if f.entries == nil {
		f.entries = make(map[int64]*RekorEntry)
	}
	f.entries[entry.LogIndex] = &entry
}

func (f *FakeVerifier) Verify(_ context.Context, logIndex int64, expectedDigest string) (*RekorEntry, error) {
	if f.Err != nil {
		return nil, f.Err
	}

	entry, ok := f.entries[logIndex]
	if !ok {
		return nil, fmt.Errorf("rekor entry not found for log index %d", logIndex)
	}

	if entry.Digest != expectedDigest {
		return nil, fmt.Errorf("digest mismatch: rekor has %q, expected %q", entry.Digest, expectedDigest)
	}

	return entry, nil
}

// FakeSearcher is a Searcher for testing. Stores entries keyed by digest.
type FakeSearcher struct {
	Err     error
	entries map[string]*RekorEntry
}

// NewFakeSearcher creates a FakeSearcher with no entries.
func NewFakeSearcher() *FakeSearcher {
	return &FakeSearcher{entries: make(map[string]*RekorEntry)}
}

// AddEntry registers a Rekor entry discoverable by digest.
func (f *FakeSearcher) AddEntry(entry RekorEntry) {
	if f.entries == nil {
		f.entries = make(map[string]*RekorEntry)
	}
	f.entries[entry.Digest] = &entry
}

func (f *FakeSearcher) Search(_ context.Context, digest string) (*RekorEntry, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	entry, ok := f.entries[digest]
	if !ok {
		return nil, fmt.Errorf("no rekor entry found for digest %s", digest)
	}
	return entry, nil
}
