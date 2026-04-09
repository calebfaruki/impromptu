package web

import (
	"bytes"
	"html/template"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

var chromaStyle = styles.Get("monokailight")
var chromaFormatter = chromahtml.New()

func highlight(code, lang string) template.HTML {
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	tokens, err := lexer.Tokenise(nil, code)
	if err != nil {
		return template.HTML("<pre>" + template.HTMLEscapeString(code) + "</pre>")
	}

	var buf bytes.Buffer
	if err := chromaFormatter.Format(&buf, chromaStyle, tokens); err != nil {
		return template.HTML("<pre>" + template.HTMLEscapeString(code) + "</pre>")
	}

	return template.HTML(buf.String())
}
