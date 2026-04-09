package pull

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestSubmitToIndexSigned(t *testing.T) {
	srv, received := mockIndexServer(t)
	defer srv.Close()

	warnings := SubmitToIndex(context.Background(), srv.URL,
		"https://github.com/alice/coder", "sha256:abc", "alice@github.com", 42)

	if len(warnings) > 0 {
		t.Errorf("expected no warnings, got: %v", warnings)
	}
	if len(*received) != 1 {
		t.Fatalf("expected 1 submission, got %d", len(*received))
	}
	sub := (*received)[0]
	if sub["source_url"] != "https://github.com/alice/coder" {
		t.Errorf("source_url: got %v", sub["source_url"])
	}
	if sub["signer_identity"] != "alice@github.com" {
		t.Errorf("signer_identity: got %v", sub["signer_identity"])
	}
	if sub["rekor_log_index"] != float64(42) {
		t.Errorf("rekor_log_index: got %v", sub["rekor_log_index"])
	}
}

func TestSubmitToIndexCodeberg(t *testing.T) {
	srv, received := mockIndexServer(t)
	defer srv.Close()

	warnings := SubmitToIndex(context.Background(), srv.URL,
		"https://codeberg.org/alice/prompts", "sha256:abc", "alice@codeberg.org", 1)

	if len(warnings) > 0 {
		t.Errorf("expected no warnings, got: %v", warnings)
	}
	if len(*received) != 1 {
		t.Fatal("expected 1 submission")
	}
}

func TestSubmitToIndexNoSigner(t *testing.T) {
	srv, received := mockIndexServer(t)
	defer srv.Close()

	warnings := SubmitToIndex(context.Background(), srv.URL,
		"https://github.com/alice/coder", "sha256:abc", "", 0)

	if len(warnings) > 0 {
		t.Errorf("expected no warnings for empty signer, got: %v", warnings)
	}
	if len(*received) != 0 {
		t.Error("empty signer should not submit")
	}
}

func TestSubmitToIndexNonAllowlistedHost(t *testing.T) {
	srv, received := mockIndexServer(t)
	defer srv.Close()

	warnings := SubmitToIndex(context.Background(), srv.URL,
		"https://gitlab.com/alice/coder", "sha256:abc", "alice@example.com", 1)

	if len(warnings) > 0 {
		t.Errorf("non-allowlisted should silently skip, got: %v", warnings)
	}
	if len(*received) != 0 {
		t.Error("non-allowlisted should not submit")
	}
}

func TestSubmitToIndexServerDown(t *testing.T) {
	warnings := SubmitToIndex(context.Background(), "http://localhost:1",
		"https://github.com/alice/coder", "sha256:abc", "alice@github.com", 1)

	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning for server down, got %d", len(warnings))
	}
	if !strings.Contains(warnings[0], "unreachable") {
		t.Errorf("warning should mention unreachable: %s", warnings[0])
	}
}

func TestSubmitToIndexEmptyURL(t *testing.T) {
	warnings := SubmitToIndex(context.Background(), "",
		"https://github.com/alice/coder", "sha256:abc", "alice@github.com", 1)
	if len(warnings) > 0 {
		t.Errorf("empty index URL should silently skip, got: %v", warnings)
	}
}
