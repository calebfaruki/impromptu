package contentcheck

import "testing"

func TestViolationError(t *testing.T) {
	tests := []struct {
		name string
		v    Violation
		want string
	}{
		{
			name: "with line and column",
			v: Violation{
				File:   "01-context.md",
				Line:   3,
				Column: 15,
				Kind:   KindUnicode,
				Reason: "banned invisible character U+200B",
			},
			want: "01-context.md:3:15: [unicode] banned invisible character U+200B",
		},
		{
			name: "without line",
			v: Violation{
				File:   "helper.py",
				Kind:   KindFiletype,
				Reason: "non-markdown file \"helper.py\" is not allowed",
			},
			want: "helper.py: [filetype] non-markdown file \"helper.py\" is not allowed",
		},
		{
			name: "empty kind",
			v: Violation{
				File:   "dir",
				Kind:   KindEmpty,
				Reason: "directory contains no markdown files",
			},
			want: "dir: [empty] directory contains no markdown files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.v.Error()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
