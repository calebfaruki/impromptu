package contentcheck

import "testing"

func TestCheckUnicode(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantCount int
	}{
		{"clean ascii", "Hello world\n", 0},
		{"clean multilingual", "Bonjour le monde\n", 0},
		{"clean CJK", "\u4f60\u597d\u4e16\u754c\n", 0},
		{"zero-width space", "hello\u200Bworld\n", 1},
		{"zero-width non-joiner", "hello\u200Cworld\n", 1},
		{"zero-width joiner", "hello\u200Dworld\n", 1},
		{"BOM mid-file", "hello\uFEFFworld\n", 1},
		{"RTL override", "hello\u202Eworld\n", 1},
		{"LTR override", "hello\u202Dworld\n", 1},
		{"RTL embedding", "hello\u202Bworld\n", 1},
		{"LTR embedding", "hello\u202Aworld\n", 1},
		{"pop directional", "hello\u202Cworld\n", 1},
		{"LTR isolate", "hello\u2066world\n", 1},
		{"RTL isolate", "hello\u2067world\n", 1},
		{"first strong isolate", "hello\u2068world\n", 1},
		{"pop directional isolate", "hello\u2069world\n", 1},
		{"cyrillic a", "hello \u0430 world\n", 1},
		{"cyrillic e", "hello \u0435 world\n", 1},
		{"cyrillic o", "hello \u043E world\n", 1},
		{"cyrillic p", "hello \u0440 world\n", 1},
		{"cyrillic c", "hello \u0441 world\n", 1},
		{"cyrillic y", "hello \u0443 world\n", 1},
		{"cyrillic x", "hello \u0445 world\n", 1},
		{"cyrillic s", "hello \u0455 world\n", 1},
		{"cyrillic i", "hello \u0456 world\n", 1},
		{"cyrillic j", "hello \u0458 world\n", 1},
		{"cyrillic h", "hello \u04BB world\n", 1},
		{"cyrillic A", "hello \u0410 world\n", 1},
		{"cyrillic B", "hello \u0412 world\n", 1},
		{"cyrillic E", "hello \u0415 world\n", 1},
		{"cyrillic K", "hello \u041A world\n", 1},
		{"cyrillic M", "hello \u041C world\n", 1},
		{"cyrillic H", "hello \u041D world\n", 1},
		{"cyrillic O", "hello \u041E world\n", 1},
		{"cyrillic P", "hello \u0420 world\n", 1},
		{"cyrillic C", "hello \u0421 world\n", 1},
		{"cyrillic T", "hello \u0422 world\n", 1},
		{"cyrillic X", "hello \u0425 world\n", 1},
		{"greek omicron", "hello \u03BF world\n", 1},
		{"greek alpha", "hello \u03B1 world\n", 1},
		{"greek Alpha", "hello \u0391 world\n", 1},
		{"greek Beta", "hello \u0392 world\n", 1},
		{"greek Epsilon", "hello \u0395 world\n", 1},
		{"greek Eta", "hello \u0397 world\n", 1},
		{"greek Iota", "hello \u0399 world\n", 1},
		{"greek Kappa", "hello \u039A world\n", 1},
		{"greek Mu", "hello \u039C world\n", 1},
		{"greek Nu", "hello \u039D world\n", 1},
		{"greek Omicron", "hello \u039F world\n", 1},
		{"greek Rho", "hello \u03A1 world\n", 1},
		{"greek Tau", "hello \u03A4 world\n", 1},
		{"greek Chi", "hello \u03A7 world\n", 1},
		{"greek Upsilon", "hello \u03A5 world\n", 1},
		{"greek Zeta", "hello \u0396 world\n", 1},
		{"multiple on same line", "h\u0435llo\u200Bworld\n", 2},
		{"violations across lines", "hello\u200B\nworld\u200C\n", 2},
		{
			"line and column reported correctly",
			"clean line\nbad \u200B char\n",
			1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckUnicode(tt.content, "test.md")
			if len(got) != tt.wantCount {
				t.Errorf("got %d violations, want %d", len(got), tt.wantCount)
				for _, v := range got {
					t.Logf("  %s", v.Error())
				}
			}
			for _, v := range got {
				if v.Kind != KindUnicode {
					t.Errorf("got kind %s, want %s", v.Kind, KindUnicode)
				}
				if v.File != "test.md" {
					t.Errorf("got file %q, want %q", v.File, "test.md")
				}
			}
		})
	}
}

func TestCheckUnicodePositionInvisible(t *testing.T) {
	content := "clean line\nbad \u200B char\n"
	got := CheckUnicode(content, "test.md")
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1", len(got))
	}
	v := got[0]
	if v.Line != 2 {
		t.Errorf("got line %d, want 2", v.Line)
	}
	if v.Column != 5 {
		t.Errorf("got column %d, want 5", v.Column)
	}
}

func TestCheckUnicodePositionHomoglyph(t *testing.T) {
	// Cyrillic а (U+0430) on line 2, after "bad " (4 bytes)
	content := "clean line\nbad \u0430 char\n"
	got := CheckUnicode(content, "test.md")
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1", len(got))
	}
	v := got[0]
	if v.Line != 2 {
		t.Errorf("got line %d, want 2", v.Line)
	}
	if v.Column != 5 {
		t.Errorf("got column %d, want 5", v.Column)
	}
}
