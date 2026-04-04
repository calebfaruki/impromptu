package contentcheck

import (
	"fmt"
	"strings"
	"unicode"
)

// bannedInvisible covers zero-width and bidirectional override characters.
var bannedInvisible = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x200B, Hi: 0x200D, Stride: 1}, // zero-width space, ZWNJ, ZWJ
		{Lo: 0x202A, Hi: 0x202E, Stride: 1}, // LTR/RTL embedding, override, pop
		{Lo: 0x2066, Hi: 0x2069, Stride: 1}, // LTR/RTL isolate, FSI, PDI
		{Lo: 0xFEFF, Hi: 0xFEFF, Stride: 1}, // BOM / zero-width no-break space
	},
}

// homoglyphs covers characters that visually mimic ASCII in common fonts.
// R16 entries MUST be sorted by Lo for unicode.Is binary search.
var homoglyphs = &unicode.RangeTable{
	R16: []unicode.Range16{
		// Greek uppercase
		{Lo: 0x0391, Hi: 0x0392, Stride: 1}, // Α -> A, Β -> B
		{Lo: 0x0395, Hi: 0x0397, Stride: 1}, // Ε -> E, Ζ -> Z, Η -> H
		{Lo: 0x0399, Hi: 0x039A, Stride: 1}, // Ι -> I, Κ -> K
		{Lo: 0x039C, Hi: 0x039D, Stride: 1}, // Μ -> M, Ν -> N
		{Lo: 0x039F, Hi: 0x039F, Stride: 1}, // Ο -> O
		{Lo: 0x03A1, Hi: 0x03A1, Stride: 1}, // Ρ -> P
		{Lo: 0x03A4, Hi: 0x03A5, Stride: 1}, // Τ -> T, Υ -> Y
		{Lo: 0x03A7, Hi: 0x03A7, Stride: 1}, // Χ -> X
		// Greek lowercase
		{Lo: 0x03B1, Hi: 0x03B1, Stride: 1}, // α -> a
		{Lo: 0x03BF, Hi: 0x03BF, Stride: 1}, // ο -> o
		// Cyrillic uppercase
		{Lo: 0x0410, Hi: 0x0410, Stride: 1}, // А -> A
		{Lo: 0x0412, Hi: 0x0412, Stride: 1}, // В -> B
		{Lo: 0x0415, Hi: 0x0415, Stride: 1}, // Е -> E
		{Lo: 0x041A, Hi: 0x041A, Stride: 1}, // К -> K
		{Lo: 0x041C, Hi: 0x041E, Stride: 1}, // М -> M, Н -> H, О -> O
		{Lo: 0x0420, Hi: 0x0422, Stride: 1}, // Р -> P, С -> C, Т -> T
		{Lo: 0x0425, Hi: 0x0425, Stride: 1}, // Х -> X
		// Cyrillic lowercase
		{Lo: 0x0430, Hi: 0x0430, Stride: 1}, // а -> a
		{Lo: 0x0435, Hi: 0x0435, Stride: 1}, // е -> e
		{Lo: 0x043E, Hi: 0x043E, Stride: 1}, // о -> o
		{Lo: 0x0440, Hi: 0x0441, Stride: 1}, // р -> p, с -> c
		{Lo: 0x0443, Hi: 0x0443, Stride: 1}, // у -> y
		{Lo: 0x0445, Hi: 0x0445, Stride: 1}, // х -> x
		{Lo: 0x0455, Hi: 0x0456, Stride: 1}, // ѕ -> s, і -> i
		{Lo: 0x0458, Hi: 0x0458, Stride: 1}, // ј -> j
		{Lo: 0x04BB, Hi: 0x04BB, Stride: 1}, // һ -> h
	},
}

// CheckUnicode scans content for banned invisible characters and homoglyphs.
// Column values are 1-based byte offsets within the line.
func CheckUnicode(content string, file string) []Violation {
	var violations []Violation
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		for j, r := range line {
			if unicode.Is(bannedInvisible, r) {
				violations = append(violations, Violation{
					File:   file,
					Line:   i + 1,
					Column: j + 1,
					Kind:   KindUnicode,
					Reason: fmt.Sprintf("banned invisible character U+%04X", r),
				})
			} else if unicode.Is(homoglyphs, r) {
				violations = append(violations, Violation{
					File:   file,
					Line:   i + 1,
					Column: j + 1,
					Kind:   KindUnicode,
					Reason: fmt.Sprintf("homoglyph character U+%04X mimics ASCII", r),
				})
			}
		}
	}
	return violations
}
