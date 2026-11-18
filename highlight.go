package main

import (
	"strings"

	"github.com/vivaui/viva/components"
	"github.com/vivaui/viva/gfx"
)

// The preview colors the code of a fenced block by language. Java (and the
// other C-like languages the lexer understands), XML and JSON each get their
// own scanner; anything else is left plain, which is what the renderer does
// when no highlighter is given.

// syntax is the palette of one appearance: the color of each class of code.
type syntax struct {
	comment  gfx.Color
	str      gfx.Color
	number   gfx.Color
	keyword  gfx.Color
	function gfx.Color
	variable gfx.Color
	key      gfx.Color
}

// lightSyntax is the palette for the light theme, close to the colors an editor
// uses on white.
func lightSyntax() syntax {
	return syntax{
		comment:  gfx.RGB24(0x6e7781),
		str:      gfx.RGB24(0x0a7d33),
		number:   gfx.RGB24(0xb25000),
		keyword:  gfx.RGB24(0xa626a4),
		function: gfx.RGB24(0x8250df),
		variable: gfx.RGB24(0x0550ae),
		key:      gfx.RGB24(0x0550ae),
	}
}

// darkSyntax is the palette for the dark theme: the same hues, lifted so they
// stay legible on a dark card.
func darkSyntax() syntax {
	return syntax{
		comment:  gfx.RGB24(0x8b949e),
		str:      gfx.RGB24(0x7ee787),
		number:   gfx.RGB24(0x79c0ff),
		keyword:  gfx.RGB24(0xff7b72),
		function: gfx.RGB24(0xd2a8ff),
		variable: gfx.RGB24(0x79c0ff),
		key:      gfx.RGB24(0x79c0ff),
	}
}

// markdownLook is the look of the preview: the theme's own, with the colors of
// highlighted code filled in. The renderer leaves them transparent, so a class
// nobody colors is drawn in the body color.
func markdownLook(theme gfx.Theme, dark bool) components.MarkdownLook {
	look := components.MarkdownLookFor(theme)
	colors := lightSyntax()
	if dark {
		colors = darkSyntax()
	}
	look.Code[components.CodeComment] = colors.comment
	look.Code[components.CodeString] = colors.str
	look.Code[components.CodeNumber] = colors.number
	look.Code[components.CodeKeyword] = colors.keyword
	look.Code[components.CodeFunction] = colors.function
	look.Code[components.CodeVariable] = colors.variable
	look.Code[components.CodeFlag] = colors.keyword
	look.Code[components.CodeKey] = colors.key
	look.Code[components.CodeAdded] = colors.str
	look.Code[components.CodeRemoved] = colors.keyword
	return look
}

// highlightCode is the MarkdownStyle.Highlight of the preview: it colors one
// line of a fenced block from the language named after the opening fence.
func highlightCode(language, line string, dst []components.MarkdownCodeRun) []components.MarkdownCodeRun {
	switch codeLanguage(language) {
	case langXML:
		return highlightXML(line, dst)
	case langJSON:
		return highlightJSON(line, dst)
	case langCLike:
		return highlightCLike(line, dst)
	}
	return dst
}

// The languages the preview colors. langNone leaves a block plain.
const (
	langNone = iota
	langCLike
	langXML
	langJSON
)

// codeLanguage maps the info string of a fenced block to one of the lexers.
// The first word is the language, as in "java title=Note".
func codeLanguage(info string) int {
	name := strings.ToLower(strings.TrimSpace(info))
	if cut := strings.IndexAny(name, " \t,;:"); cut >= 0 {
		name = name[:cut]
	}
	switch name {
	case "xml", "html", "htm", "xhtml", "svg", "xsl", "xslt", "plist",
		"csproj", "xaml", "rss", "atom", "wsdl", "pom":
		return langXML
	case "json", "jsonc", "json5":
		return langJSON
	case "java", "kt", "kotlin", "scala", "groovy", "gradle", "c", "h", "cc",
		"cpp", "cxx", "hpp", "hh", "cs", "csharp", "go", "golang", "js",
		"javascript", "jsx", "mjs", "ts", "typescript", "tsx", "rust", "rs",
		"swift", "php", "dart", "m", "mm", "css", "scss", "less", "sql":
		return langCLike
	}
	return langNone
}

// run appends a colored stretch to dst, dropping an empty one.
func run(dst []components.MarkdownCodeRun, start, end int, class components.MarkdownCodeClass) []components.MarkdownCodeRun {
	if end <= start {
		return dst
	}
	return append(dst, components.MarkdownCodeRun{Start: start, End: end, Class: class})
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isIdentPart(c byte) bool { return isIdentStart(c) || isDigit(c) }

// scanIdent returns the end of the identifier that starts at i.
func scanIdent(line string, i int) int {
	for i < len(line) && isIdentPart(line[i]) {
		i++
	}
	return i
}

// scanQuoted returns the end of the quoted literal that starts at i, past its
// closing quote, or the end of the line for one that is not closed.
func scanQuoted(line string, i int) int {
	quote := line[i]
	for j := i + 1; j < len(line); j++ {
		switch line[j] {
		case '\\':
			j++
		case quote:
			return j + 1
		}
	}
	return len(line)
}

// scanNumber returns the end of the number that starts at i, taking in the
// letters of a hexadecimal literal and the sign of an exponent.
func scanNumber(line string, i int) int {
	for i < len(line) {
		c := line[i]
		switch {
		case isIdentPart(c), c == '.':
			i++
		case (c == '+' || c == '-') && i > 0 && (line[i-1] == 'e' || line[i-1] == 'E'):
			i++
		default:
			return i
		}
	}
	return i
}

// nextNonSpace is the byte after the spaces that follow at.
func nextNonSpace(line string, at int) byte {
	for at < len(line) && (line[at] == ' ' || line[at] == '\t') {
		at++
	}
	if at < len(line) {
		return line[at]
	}
	return 0
}

// blockCommentEnd is the end of the /* */ comment that starts at i, or the end
// of the line when it is not closed on it.
func blockCommentEnd(line string, i int) int {
	if end := strings.Index(line[i+2:], "*/"); end >= 0 {
		return i + 2 + end + 2
	}
	return len(line)
}

// cKeywords are the words of the C-like languages the lexer colors, in one set:
// a word that is a keyword anywhere is colored everywhere, which is what a note
// wants and what keeps the table small.
var cKeywords = map[string]bool{
	"abstract": true, "any": true, "as": true, "assert": true, "async": true,
	"await": true, "bool": true, "boolean": true, "break": true, "byte": true,
	"case": true, "catch": true, "char": true, "class": true, "const": true,
	"constexpr": true, "continue": true, "data": true, "def": true,
	"default": true, "defer": true, "delete": true, "do": true, "double": true,
	"dyn": true, "elif": true, "else": true, "enum": true, "export": true,
	"extends": true, "extern": true, "false": true, "final": true,
	"finally": true, "float": true, "fn": true, "for": true, "func": true,
	"function": true, "go": true, "goto": true, "if": true, "impl": true,
	"implements": true, "import": true, "in": true, "instanceof": true,
	"int": true, "interface": true, "internal": true, "is": true, "lambda": true,
	"let": true, "long": true, "map": true, "match": true, "module": true,
	"mut": true, "native": true, "new": true, "nil": true, "null": true,
	"object": true, "operator": true, "package": true, "private": true,
	"protected": true, "protocol": true, "public": true, "raise": true,
	"readonly": true, "record": true, "ref": true, "return": true, "sealed": true,
	"short": true, "sizeof": true, "static": true, "strictfp": true,
	"struct": true, "super": true, "switch": true, "synchronized": true,
	"this": true, "throw": true, "throws": true, "trait": true, "transient": true,
	"true": true, "try": true, "type": true, "typeof": true, "union": true,
	"unsigned": true, "use": true, "using": true, "var": true, "virtual": true,
	"void": true, "volatile": true, "where": true, "while": true, "with": true,
	"yield": true,
}

// cLiterals are the words that read as a value rather than as a keyword.
var cLiterals = map[string]bool{"true": true, "false": true, "null": true, "nil": true}

// highlightCLike colors one line of Java, Kotlin, Go, C, JavaScript and the
// languages that look like them: comments, literals, numbers, keywords,
// annotations and the names of the functions that are called.
func highlightCLike(line string, dst []components.MarkdownCodeRun) []components.MarkdownCodeRun {
	// A line that begins with a star is the inside of a block comment, which is
	// how a doc comment is written and the one case a line can tell on its own.
	if trimmed := strings.TrimLeft(line, " \t"); strings.HasPrefix(trimmed, "*") {
		return run(dst, len(line)-len(trimmed), len(line), components.CodeComment)
	}
	for i := 0; i < len(line); {
		c := line[i]
		switch {
		case c == '/' && i+1 < len(line) && line[i+1] == '/':
			return run(dst, i, len(line), components.CodeComment)
		case c == '/' && i+1 < len(line) && line[i+1] == '*':
			end := blockCommentEnd(line, i)
			dst, i = run(dst, i, end, components.CodeComment), end
		case c == '"' || c == '\'' || c == '`':
			end := scanQuoted(line, i)
			dst, i = run(dst, i, end, components.CodeString), end
		case c == '@' && i+1 < len(line) && isIdentStart(line[i+1]):
			end := scanIdent(line, i+1)
			dst, i = run(dst, i, end, components.CodeFlag), end
		case isDigit(c):
			end := scanNumber(line, i)
			dst, i = run(dst, i, end, components.CodeNumber), end
		case isIdentStart(c):
			end := scanIdent(line, i)
			word := line[i:end]
			switch {
			case cLiterals[word]:
				dst = run(dst, i, end, components.CodeNumber)
			case cKeywords[word]:
				dst = run(dst, i, end, components.CodeKeyword)
			case nextNonSpace(line, end) == '(':
				dst = run(dst, i, end, components.CodeFunction)
			}
			i = end
		default:
			i++
		}
	}
	return dst
}

// highlightXML colors one line of XML, HTML and their relatives: comments, the
// markers and names of tags, attribute names and their values.
func highlightXML(line string, dst []components.MarkdownCodeRun) []components.MarkdownCodeRun {
	inTag, first := false, false
	for i := 0; i < len(line); {
		if !inTag {
			at := strings.IndexByte(line[i:], '<')
			if at < 0 {
				return dst
			}
			i += at
			if strings.HasPrefix(line[i:], "<!--") {
				if end := strings.Index(line[i+4:], "-->"); end >= 0 {
					end += i + 7
					dst, i = run(dst, i, end, components.CodeComment), end
					continue
				}
				return run(dst, i, len(line), components.CodeComment)
			}
			end := i + 1
			if end < len(line) && (line[end] == '?' || line[end] == '/' || line[end] == '!') {
				end++
			}
			dst, i = run(dst, i, end, components.CodeKeyword), end
			inTag, first = true, true
			continue
		}
		c := line[i]
		switch {
		case c == '?' && i+1 < len(line) && line[i+1] == '>':
			dst, i, inTag = run(dst, i, i+2, components.CodeKeyword), i+2, false
		case c == '/' && i+1 < len(line) && line[i+1] == '>':
			dst, i, inTag = run(dst, i, i+2, components.CodeKeyword), i+2, false
		case c == '>':
			dst, i, inTag = run(dst, i, i+1, components.CodeKeyword), i+1, false
		case c == '"' || c == '\'':
			end := scanQuoted(line, i)
			dst, i = run(dst, i, end, components.CodeString), end
		case isIdentStart(c):
			end := scanIdent(line, i)
			class := components.CodeVariable
			if first {
				class, first = components.CodeKeyword, false
			}
			dst, i = run(dst, i, end, class), end
		default:
			i++
		}
	}
	return dst
}

// highlightJSON colors one line of JSON: comments, the keys of an object, the
// strings and numbers that are values, and the three literals.
func highlightJSON(line string, dst []components.MarkdownCodeRun) []components.MarkdownCodeRun {
	for i := 0; i < len(line); {
		c := line[i]
		switch {
		case c == '/' && i+1 < len(line) && line[i+1] == '/':
			return run(dst, i, len(line), components.CodeComment)
		case c == '/' && i+1 < len(line) && line[i+1] == '*':
			end := blockCommentEnd(line, i)
			dst, i = run(dst, i, end, components.CodeComment), end
		case c == '"':
			end := scanQuoted(line, i)
			class := components.CodeString
			if nextNonSpace(line, end) == ':' {
				class = components.CodeKey
			}
			dst, i = run(dst, i, end, class), end
		case isDigit(c) || (c == '-' && i+1 < len(line) && isDigit(line[i+1])):
			end := scanNumber(line, i)
			dst, i = run(dst, i, end, components.CodeNumber), end
		case isIdentStart(c):
			end := scanIdent(line, i)
			if word := line[i:end]; word == "true" || word == "false" || word == "null" {
				dst = run(dst, i, end, components.CodeKeyword)
			}
			i = end
		default:
			i++
		}
	}
	return dst
}
