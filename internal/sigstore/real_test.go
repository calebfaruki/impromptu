package sigstore

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// --- Test helpers ---

func testCertWithEmail(t *testing.T, email string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		EmailAddresses: []string{email},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return base64.StdEncoding.EncodeToString(pemBlock)
}

func testCertWithURI(t *testing.T, uri string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(uri)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		URIs:         []*url.URL{u},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return base64.StdEncoding.EncodeToString(pemBlock)
}

func makeHashedRekordBody(digest, certBase64 string) string {
	rekord := hashedRekordBody{
		APIVersion: "0.0.2",
		Kind:       "hashedrekord",
	}
	rekord.Spec.Data.Hash.Algorithm = "sha256"
	rekord.Spec.Data.Hash.Value = digest
	rekord.Spec.Signature.PublicKey.Content = certBase64
	data, _ := json.Marshal(rekord)
	return base64.StdEncoding.EncodeToString(data)
}

func makeRekorResponse(uuid string, logIndex int64, body string) map[string]rekorEntryBody {
	return map[string]rekorEntryBody{
		uuid: {Body: body, LogIndex: logIndex, IntegratedTime: time.Now().Unix()},
	}
}

// --- extractSignerIdentity tests ---

func TestExtractSignerIdentityEmail(t *testing.T) {
	certB64 := testCertWithEmail(t, "alice@github.com")
	identity, err := extractSignerIdentity(certB64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if identity != "alice@github.com" {
		t.Errorf("got %q, want alice@github.com", identity)
	}
}

func TestExtractSignerIdentityURI(t *testing.T) {
	uri := "https://github.com/alice/repo/.github/workflows/release.yml@refs/heads/main"
	certB64 := testCertWithURI(t, uri)
	identity, err := extractSignerIdentity(certB64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if identity != uri {
		t.Errorf("got %q, want %q", identity, uri)
	}
}

func TestExtractSignerIdentityErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", "empty certificate"},
		{"bad base64", "not-base64!!!", "decoding certificate"},
		{"no PEM", base64.StdEncoding.EncodeToString([]byte("not a cert")), "no PEM block"},
		{"bad DER", base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("bad")})), "parsing certificate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractSignerIdentity(tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
			if !contains(err.Error(), tt.want) {
				t.Errorf("error %q should contain %q", err.Error(), tt.want)
			}
		})
	}
}

func TestExtractSignerIdentityNoSAN(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	certB64 := base64.StdEncoding.EncodeToString(pemBlock)

	_, err := extractSignerIdentity(certB64)
	if err == nil {
		t.Fatal("expected error for cert with no SANs")
	}
	if !contains(err.Error(), "no email or URI") {
		t.Errorf("error %q should mention no email or URI", err.Error())
	}
}

// --- RekorVerifier tests ---

func TestRekorVerifyValid(t *testing.T) {
	certB64 := testCertWithEmail(t, "alice@github.com")
	body := makeHashedRekordBody("abcd1234", certB64)
	resp := makeRekorResponse("uuid1", 42, body)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	v := NewRekorVerifier(srv.URL)
	entry, err := v.Verify(context.Background(), 42, "sha256:abcd1234")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if entry.LogIndex != 42 {
		t.Errorf("LogIndex: got %d, want 42", entry.LogIndex)
	}
	if entry.Digest != "sha256:abcd1234" {
		t.Errorf("Digest: got %q", entry.Digest)
	}
	if entry.SignerIdentity != "alice@github.com" {
		t.Errorf("SignerIdentity: got %q", entry.SignerIdentity)
	}
}

func TestRekorVerifyDigestMismatch(t *testing.T) {
	certB64 := testCertWithEmail(t, "alice@github.com")
	body := makeHashedRekordBody("abcd1234", certB64)
	resp := makeRekorResponse("uuid1", 42, body)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	v := NewRekorVerifier(srv.URL)
	_, err := v.Verify(context.Background(), 42, "sha256:different")
	if err == nil {
		t.Fatal("expected digest mismatch error")
	}
	if !contains(err.Error(), "digest mismatch") {
		t.Errorf("error %q should contain 'digest mismatch'", err.Error())
	}
}

func TestRekorVerifyEntryNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	v := NewRekorVerifier(srv.URL)
	_, err := v.Verify(context.Background(), 999, "sha256:abc")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !contains(err.Error(), "not found") {
		t.Errorf("error %q should contain 'not found'", err.Error())
	}
}

func TestRekorVerifyHTTPErrors(t *testing.T) {
	codes := []int{http.StatusNotFound, http.StatusInternalServerError}
	for _, code := range codes {
		t.Run(fmt.Sprintf("HTTP_%d", code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer srv.Close()

			v := NewRekorVerifier(srv.URL)
			_, err := v.Verify(context.Background(), 1, "sha256:abc")
			if err == nil {
				t.Fatalf("expected error for HTTP %d", code)
			}
			if !contains(err.Error(), fmt.Sprintf("%d", code)) {
				t.Errorf("error %q should contain status code %d", err.Error(), code)
			}
		})
	}
}

func TestRekorVerifyNonHashedRekord(t *testing.T) {
	rekord := map[string]any{
		"apiVersion": "0.0.1",
		"kind":       "intoto",
		"spec":       map[string]any{},
	}
	data, _ := json.Marshal(rekord)
	body := base64.StdEncoding.EncodeToString(data)
	resp := makeRekorResponse("uuid1", 1, body)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	v := NewRekorVerifier(srv.URL)
	_, err := v.Verify(context.Background(), 1, "sha256:abc")
	if err == nil {
		t.Fatal("expected error for non-hashedrekord")
	}
	if !contains(err.Error(), "unsupported entry kind") {
		t.Errorf("error %q should mention unsupported kind", err.Error())
	}
}

// --- RekorSearcher tests ---

func TestSearchFound(t *testing.T) {
	certB64 := testCertWithEmail(t, "bob@github.com")
	body := makeHashedRekordBody("deadbeef", certB64)
	entryResp := makeRekorResponse("uuid-abc", 99, body)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/index/retrieve" {
			json.NewEncoder(w).Encode([]string{"uuid-abc"})
			return
		}
		if r.URL.Path == "/api/v1/log/entries/uuid-abc" {
			json.NewEncoder(w).Encode(entryResp)
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	s := NewRekorSearcher(srv.URL)
	entry, err := s.Search(context.Background(), "sha256:deadbeef")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if entry.LogIndex != 99 {
		t.Errorf("LogIndex: got %d, want 99", entry.LogIndex)
	}
	if entry.SignerIdentity != "bob@github.com" {
		t.Errorf("SignerIdentity: got %q", entry.SignerIdentity)
	}
}

func TestSearchNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]string{})
	}))
	defer srv.Close()

	s := NewRekorSearcher(srv.URL)
	_, err := s.Search(context.Background(), "sha256:nonexistent")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !contains(err.Error(), "no rekor entry found") {
		t.Errorf("error %q should contain 'no rekor entry found'", err.Error())
	}
}

func TestSearchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := NewRekorSearcher(srv.URL)
	_, err := s.Search(context.Background(), "sha256:abc")
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestSearchMultipleUUIDs(t *testing.T) {
	certB64 := testCertWithEmail(t, "carol@github.com")
	body := makeHashedRekordBody("aabb", certB64)
	entryResp := makeRekorResponse("first-uuid", 10, body)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/index/retrieve" {
			json.NewEncoder(w).Encode([]string{"first-uuid", "second-uuid"})
			return
		}
		if r.URL.Path == "/api/v1/log/entries/first-uuid" {
			json.NewEncoder(w).Encode(entryResp)
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	s := NewRekorSearcher(srv.URL)
	entry, err := s.Search(context.Background(), "sha256:aabb")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if entry.LogIndex != 10 {
		t.Errorf("should use first UUID, got LogIndex %d", entry.LogIndex)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
