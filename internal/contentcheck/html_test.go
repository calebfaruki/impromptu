package contentcheck

import "testing"

func TestCheckHTML(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantCount int
	}{
		{"clean markdown", "# Hello\n\nSome text\n", 0},
		{"emphasis and links", "**bold** and [link](url)\n", 0},
		{"angle bracket not tag", "3 < 5 is true\n", 0},
		{"angle bracket before digit", "use <3 emoji\n", 0},
		{"angle bracket before space", "a < b\n", 0},
		{
			"html in backtick fence",
			"```html\n<div>ok</div>\n```\n",
			0,
		},
		{
			"html in tilde fence",
			"~~~html\n<div>ok</div>\n~~~\n",
			0,
		},
		{
			"html in four backtick fence",
			"````\n<div>ok</div>\n````\n",
			0,
		},
		{
			"nested fences different chars",
			"````\n```\n<div>ok</div>\n```\n````\n",
			0,
		},
		{
			"unclosed fence extends to end",
			"```\n<div>inside unclosed fence</div>\n",
			0,
		},
		{
			"yaml frontmatter skipped",
			"---\ntitle: <test>\nauthor: me\n---\n# Hello\n",
			0,
		},
		{
			"yaml frontmatter with dots closer",
			"---\ntitle: <test>\n...\n# Hello\n",
			0,
		},
		{
			"raw div tag",
			"# Title\n\n<div>hidden</div>\n",
			1,
		},
		{
			"raw script tag",
			"text\n<script>alert(1)</script>\n",
			1,
		},
		{
			"self-closing img",
			"text\n<img src=\"x\">\n",
			1,
		},
		{
			"closing tag alone",
			"text\n</div>\n",
			1,
		},
		{
			"html after fence closes",
			"```\nfenced\n```\n<div>outside</div>\n",
			1,
		},
		{
			"multiple html tags same file",
			"<div>one</div>\n<script>two</script>\n",
			2,
		},
		{
			"indented fence 3 spaces ok",
			"   ```\n<div>inside</div>\n   ```\n",
			0,
		},
		{
			"indented 4 spaces not a fence",
			"    ```\n<div>not fenced</div>\n    ```\n",
			1,
		},
		{
			"backtick in backtick info string rejects fence",
			"``` foo`bar\n<div>not fenced</div>\n```\n",
			1,
		},
		{
			"tilde fence needs matching close",
			"~~~\n<div>inside</div>\n```\n",
			0,
		},
		{
			"shorter close does not close fence",
			"````\n<div>inside</div>\n```\n",
			0,
		},
		{
			"close fence with trailing text does not close",
			"```\n<div>inside</div>\n```text\n<div>still inside</div>\n```\n",
			0,
		},
		{
			"frontmatter with html deep in body",
			"---\ntitle: test\nauthor: <b>me</b>\n---\n# Hello\n",
			0,
		},
		{
			"frontmatter dots with html deep in body",
			"---\ntitle: <em>test</em>\ndesc: <b>bold</b>\n...\n# Hello\n",
			0,
		},
		{
			"tag boundary a lowercase",
			"<a href=\"#\">link</a>\n",
			1,
		},
		{
			"tag boundary z lowercase",
			"<z>custom</z>\n",
			1,
		},
		{
			"tag boundary A uppercase",
			"<A>old style</A>\n",
			1,
		},
		{
			"tag boundary Z uppercase",
			"<Z>weird</Z>\n",
			1,
		},
		{
			"angle bracket before backtick not tag",
			"use <` code\n",
			0,
		},
		{
			"angle bracket before at not tag",
			"email <@user\n",
			0,
		},
		{
			"angle bracket before bracket not tag",
			"array <[1,2]\n",
			0,
		},
		{
			"line ending with angle bracket",
			"3 <\n",
			0,
		},
		{
			"line ending with slash after angle",
			"foo </\n",
			0,
		},
		{
			"minimal tag",
			"<b>",
			1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckHTML(tt.content, "test.md")
			if len(got) != tt.wantCount {
				t.Errorf("got %d violations, want %d", len(got), tt.wantCount)
				for _, v := range got {
					t.Logf("  %s", v.Error())
				}
			}
			for _, v := range got {
				if v.Kind != KindHTML {
					t.Errorf("got kind %s, want %s", v.Kind, KindHTML)
				}
			}
		})
	}
}

func TestCheckHTMLPosition(t *testing.T) {
	content := "clean line\n  <div>bad</div>\n"
	got := CheckHTML(content, "test.md")
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1", len(got))
	}
	v := got[0]
	if v.Line != 2 {
		t.Errorf("got line %d, want 2", v.Line)
	}
	if v.Column != 3 {
		t.Errorf("got column %d, want 3", v.Column)
	}
}
