package pull

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/calebfaruki/impromptu/internal/sigstore"
)

func mockIndexServer(t *testing.T) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var received []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		received = append(received, body)
		w.WriteHeader(http.StatusOK)
	}))
	return srv, &received
}

func signedSearcher(digest, identity string, logIndex int64) *sigstore.FakeSearcher {
	s := sigstore.NewFakeSearcher()
	s.AddEntry(sigstore.RekorEntry{LogIndex: logIndex, Digest: digest, SignerIdentity: identity})
	return s
}

func TestMaybeIndexSignedPublicGitHub(t *testing.T) {
	srv, received := mockIndexServer(t)
	defer srv.Close()

	warnings := MaybeIndex(context.Background(), srv.URL,
		"https://github.com/alice/coder", "sha256:abc",
		signedSearcher("sha256:abc", "alice@github.com", 42))

	if len(warnings) > 0 {
		t.Errorf("expected no warnings, got: %v", warnings)
	}
	if len(*received) != 1 {
		t.Fatalf("expected 1 index submission, got %d", len(*received))
	}
	if (*received)[0]["source_url"] != "https://github.com/alice/coder" {
		t.Errorf("source_url: got %v", (*received)[0]["source_url"])
	}
}

func TestMaybeIndexSignedPublicCodeberg(t *testing.T) {
	srv, received := mockIndexServer(t)
	defer srv.Close()

	warnings := MaybeIndex(context.Background(), srv.URL,
		"https://codeberg.org/alice/prompts", "sha256:abc",
		signedSearcher("sha256:abc", "alice@codeberg.org", 1))

	if len(warnings) > 0 {
		t.Errorf("expected no warnings, got: %v", warnings)
	}
	if len(*received) != 1 {
		t.Fatal("expected 1 submission")
	}
}

func TestMaybeIndexUnsigned(t *testing.T) {
	srv, received := mockIndexServer(t)
	defer srv.Close()

	// Empty searcher -- no entries, Search returns error
	warnings := MaybeIndex(context.Background(), srv.URL,
		"https://github.com/alice/coder", "sha256:abc",
		sigstore.NewFakeSearcher())

	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if !strings.Contains(warnings[0], "unsigned") {
		t.Errorf("warning should mention unsigned: %s", warnings[0])
	}
	if len(*received) != 0 {
		t.Error("unsigned should not submit to index")
	}
}

func TestMaybeIndexNonAllowlistedHost(t *testing.T) {
	srv, received := mockIndexServer(t)
	defer srv.Close()

	warnings := MaybeIndex(context.Background(), srv.URL,
		"https://gitlab.com/alice/coder", "sha256:abc",
		signedSearcher("sha256:abc", "alice@example.com", 1))

	if len(warnings) > 0 {
		t.Errorf("non-allowlisted should silently skip, got: %v", warnings)
	}
	if len(*received) != 0 {
		t.Error("non-allowlisted should not submit")
	}
}

func TestMaybeIndexServerDown(t *testing.T) {
	warnings := MaybeIndex(context.Background(), "http://localhost:1",
		"https://github.com/alice/coder", "sha256:abc",
		signedSearcher("sha256:abc", "alice@github.com", 1))

	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning for server down, got %d", len(warnings))
	}
	if !strings.Contains(warnings[0], "unreachable") {
		t.Errorf("warning should mention unreachable: %s", warnings[0])
	}
}

func TestMaybeIndexIdempotent(t *testing.T) {
	srv, received := mockIndexServer(t)
	defer srv.Close()

	s := signedSearcher("sha256:abc", "alice@github.com", 1)
	MaybeIndex(context.Background(), srv.URL, "https://github.com/alice/coder", "sha256:abc", s)
	MaybeIndex(context.Background(), srv.URL, "https://github.com/alice/coder", "sha256:abc", s)

	if len(*received) != 2 {
		t.Errorf("expected 2 submissions (server handles idempotency), got %d", len(*received))
	}
}

func TestMaybeIndexEmptyURL(t *testing.T) {
	warnings := MaybeIndex(context.Background(), "",
		"https://github.com/alice/coder", "sha256:abc", nil)
	if len(warnings) > 0 {
		t.Errorf("empty index URL should silently skip, got: %v", warnings)
	}
}
