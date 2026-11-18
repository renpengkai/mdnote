package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/vivaui/viva/markdown"
)

// The exports besides Word: a standalone HTML page and plain text. Both walk the
// same parsed document the preview draws, so a note reads the same wherever it
// goes.

const htmlHead = `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>MDNote</title>
<style>
:root { color-scheme: light dark; }
body { margin: 0 auto; max-width: 46rem; padding: 2.5rem 1.25rem 5rem;
  font: 16px/1.7 -apple-system, "Segoe UI", Roboto, "Helvetica Neue", "PingFang SC", "Microsoft YaHei", sans-serif;
  color: #1f2328; background: #fff; }
h1, h2, h3, h4, h5, h6 { line-height: 1.3; margin: 1.6em 0 .6em; }
h1 { font-size: 1.9em; border-bottom: 1px solid #e5e7eb; padding-bottom: .3em; }
h2 { font-size: 1.5em; border-bottom: 1px solid #e5e7eb; padding-bottom: .25em; }
h3 { font-size: 1.25em; }
p { margin: .7em 0; }
a { color: #0b57d0; }
code { font-family: ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", monospace;
  font-size: .92em; background: #f0f1f3; padding: .15em .35em; border-radius: 4px; }
pre { background: #f6f8fa; border: 1px solid #e5e7eb; border-radius: 8px; padding: 1em; overflow: auto; }
pre code { background: none; padding: 0; }
blockquote { margin: .8em 0; padding: .2em 1em; border-left: 4px solid #d0d7de; color: #57606a; }
hr { border: none; border-top: 1px solid #e5e7eb; margin: 1.8em 0; }
table { border-collapse: collapse; margin: 1em 0; width: 100%; }
th, td { border: 1px solid #d0d7de; padding: .4em .7em; text-align: left; }
th { background: #f6f8fa; }
ul, ol { padding-left: 1.6em; }
li { margin: .25em 0; }
@media (prefers-color-scheme: dark) {
  body { color: #e6edf3; background: #0d1117; }
  h1, h2 { border-color: #30363d; }
  a { color: #6cb6ff; }
  code { background: #21262d; }
  pre { background: #161b22; border-color: #30363d; }
  blockquote { border-color: #3d444d; color: #9198a1; }
  hr { border-color: #30363d; }
  th, td { border-color: #30363d; }
  th { background: #161b22; }
}
</style>
</head>
<body>
`

// writeHTML renders Markdown as one self-contained HTML page.
func writeHTML(path, text string) error {
	var doc markdown.Document
	doc.Parse(text)
	var b strings.Builder
	b.WriteString(htmlHead)
	list := 0 // 0 none, 1 ul, 2 ol
	closeList := func() {
		switch list {
		case 1:
			b.WriteString("</ul>\n")
		case 2:
			b.WriteString("</ol>\n")
		}
		list = 0
	}
	for i := range doc.Blocks {
		blk := &doc.Blocks[i]
		switch blk.Kind {
		case markdown.Heading:
			closeList()
			tag := "h" + strconv.Itoa(min(max(blk.Level, 1), 6))
			b.WriteString("<" + tag + ">")
			htmlRuns(&b, &doc, blk)
			b.WriteString("</" + tag + ">\n")
		case markdown.Code:
			closeList()
			b.WriteString("<pre><code>")
			b.WriteString(htmlEscape(blk.Text))
			b.WriteString("</code></pre>\n")
		case markdown.Rule:
			closeList()
			b.WriteString("<hr>\n")
		case markdown.Table:
			closeList()
			htmlTable(&b, &doc, blk)
		case markdown.Item:
			want := 1
			if blk.Number > 0 {
				want = 2
			}
			if list != want {
				closeList()
				if want == 1 {
					b.WriteString("<ul>\n")
				} else {
					b.WriteString("<ol>\n")
				}
				list = want
			}
			b.WriteString("<li>")
			htmlRuns(&b, &doc, blk)
			b.WriteString("</li>\n")
		default:
			closeList()
			if blk.Quote > 0 {
				b.WriteString("<blockquote>\n")
			}
			b.WriteString("<p>")
			htmlRuns(&b, &doc, blk)
			b.WriteString("</p>\n")
			if blk.Quote > 0 {
				b.WriteString("</blockquote>\n")
			}
		}
	}
	closeList()
	b.WriteString("</body>\n</html>\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// htmlRuns writes the inline spans of a block.
func htmlRuns(b *strings.Builder, doc *markdown.Document, blk *markdown.Block) {
	htmlSpans(b, doc.Spans(blk))
}

// htmlSpans writes inline spans, keeping their styles and links.
func htmlSpans(b *strings.Builder, spans []markdown.Span) {
	for _, sp := range spans {
		if sp.Image != "" {
			b.WriteString(`<img src="` + htmlEscape(sp.Image) + `" alt="` + htmlEscape(sp.Text) + `">`)
			continue
		}
		text := htmlEscape(sp.Text)
		if sp.Style&markdown.Mono != 0 {
			text = "<code>" + text + "</code>"
		}
		if sp.Style&markdown.Bold != 0 {
			text = "<strong>" + text + "</strong>"
		}
		if sp.Style&markdown.Italic != 0 {
			text = "<em>" + text + "</em>"
		}
		if sp.Style&markdown.Strike != 0 {
			text = "<del>" + text + "</del>"
		}
		if sp.Link != "" {
			text = `<a href="` + htmlEscape(sp.Link) + `">` + text + `</a>`
		}
		b.WriteString(text)
	}
}

// htmlTable writes a pipe table as an HTML table.
func htmlTable(b *strings.Builder, doc *markdown.Document, blk *markdown.Block) {
	cols := blk.Columns
	if cols <= 0 {
		return
	}
	cells := doc.Cells(blk)
	b.WriteString("<table>\n")
	for i := 0; i < len(cells); i += cols {
		tag := "td"
		if i == 0 {
			tag = "th"
		}
		b.WriteString("<tr>")
		for j := 0; j < cols && i+j < len(cells); j++ {
			b.WriteString("<" + tag + ">")
			htmlSpans(b, doc.CellSpans(cells[i+j]))
			b.WriteString("</" + tag + ">")
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</table>\n")
}

// writeText renders Markdown as plain text, dropping the markers a reader does
// not need.
func writeText(path, text string) error {
	var doc markdown.Document
	doc.Parse(text)
	var b strings.Builder
	for i := range doc.Blocks {
		blk := &doc.Blocks[i]
		switch blk.Kind {
		case markdown.Heading:
			b.WriteString(strings.TrimSpace(plainRuns(&doc, blk)) + "\n\n")
		case markdown.Code:
			b.WriteString(blk.Text + "\n\n")
		case markdown.Rule:
			b.WriteString("----------\n\n")
		case markdown.Table:
			cols := blk.Columns
			cells := doc.Cells(blk)
			for r := 0; r < len(cells); r += cols {
				var line []string
				for j := 0; j < cols && r+j < len(cells); j++ {
					line = append(line, strings.TrimSpace(plainSpans(doc.CellSpans(cells[r+j]))))
				}
				b.WriteString(strings.Join(line, " | ") + "\n")
			}
			b.WriteString("\n")
		case markdown.Item:
			marker := "- "
			if blk.Number > 0 {
				marker = strconv.Itoa(blk.Number) + ". "
			}
			b.WriteString(strings.Repeat("  ", blk.Level) + marker + strings.TrimSpace(plainRuns(&doc, blk)) + "\n")
		default:
			if blk.Quote > 0 {
				for _, line := range strings.Split(plainRuns(&doc, blk), "\n") {
					b.WriteString("> " + line + "\n")
				}
			} else {
				b.WriteString(plainRuns(&doc, blk) + "\n")
			}
			b.WriteString("\n")
		}
	}
	return os.WriteFile(path, []byte(strings.TrimRight(b.String(), "\n")+"\n"), 0o644)
}

// plainRuns is the text of a block's spans with no markup.
func plainRuns(doc *markdown.Document, blk *markdown.Block) string {
	return plainSpans(doc.Spans(blk))
}

// plainSpans joins the text of spans, appending the destination of a link whose
// text does not already show it.
func plainSpans(spans []markdown.Span) string {
	var b strings.Builder
	for _, sp := range spans {
		if sp.Image != "" {
			b.WriteString(sp.Text)
			continue
		}
		b.WriteString(sp.Text)
		if sp.Link != "" && sp.Link != sp.Text {
			b.WriteString(" (" + sp.Link + ")")
		}
	}
	return b.String()
}

// htmlEscape quotes text for HTML content or an attribute value.
func htmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}