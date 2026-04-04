package oci

import "testing"

func TestComputeDigest(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{
			"hello",
			[]byte("hello"),
			"sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		},
		{
			"empty",
			[]byte(""),
			"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeDigest(tt.input)
			if string(got) != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComputeDigestDeterministic(t *testing.T) {
	data := []byte("deterministic test data")
	d1 := ComputeDigest(data)
	d2 := ComputeDigest(data)
	if d1 != d2 {
		t.Errorf("not deterministic: %q != %q", d1, d2)
	}
}

func TestDigestValidate(t *testing.T) {
	tests := []struct {
		name    string
		digest  Digest
		wantErr bool
	}{
		{"valid", Digest("sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"), false},
		{"empty", Digest(""), true},
		{"no prefix", Digest("2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"), true},
		{"wrong prefix", Digest("md5:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"), true},
		{"short hex", Digest("sha256:abcdef"), true},
		{"uppercase hex", Digest("sha256:2CF24DBA5FB0A30E26E83B2AC5B9E29E1B161E5C1FA7425E73043362938B9824"), true},
		{"non-hex chars", Digest("sha256:zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.digest.Validate()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDigestHex(t *testing.T) {
	d := Digest("sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	want := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	if got := d.Hex(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDigestString(t *testing.T) {
	d := Digest("sha256:abc123")
	if got := d.String(); got != "sha256:abc123" {
		t.Errorf("got %q, want %q", got, "sha256:abc123")
	}
}
