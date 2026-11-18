package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/vivaui/viva/markdown"
)

// A .docx is a zip of XML parts. MDNote writes the smallest set Word accepts —
// the content types, the package relationships, the document and its styles —
// and reads the document part back, so notes move between Markdown and Word
// without a library and without growing the program.

const docxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
	`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
	`</Types>`

const docxRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
	`</Relationships>`

const docxStyles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
	`<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:cs="Calibri"/>` +
	`<w:sz w:val="22"/><w:szCs w:val="22"/></w:rPr></w:rPrDefault>` +
	`<w:pPrDefault><w:pPr><w:spacing w:after="140" w:line="276" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults>` +
	`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/>` +
	`<w:pPr><w:keepNext/><w:outlineLvl w:val="0"/><w:spacing w:before="280" w:after="120"/></w:pPr>` +
	`<w:rPr><w:b/><w:sz w:val="40"/><w:szCs w:val="40"/></w:rPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/><w:basedOn w:val="Normal"/>` +
	`<w:pPr><w:keepNext/><w:outlineLvl w:val="1"/><w:spacing w:before="240" w:after="100"/></w:pPr>` +
	`<w:rPr><w:b/><w:sz w:val="32"/><w:szCs w:val="32"/></w:rPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Heading3"><w:name w:val="heading 3"/><w:basedOn w:val="Normal"/>` +
	`<w:pPr><w:keepNext/><w:outlineLvl w:val="2"/><w:spacing w:before="200" w:after="80"/></w:pPr>` +
	`<w:rPr><w:b/><w:sz w:val="26"/><w:szCs w:val="26"/></w:rPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Heading4"><w:name w:val="heading 4"/><w:basedOn w:val="Normal"/>` +
	`<w:pPr><w:keepNext/><w:outlineLvl w:val="3"/><w:spacing w:before="180" w:after="60"/></w:pPr>` +
	`<w:rPr><w:b/><w:i/><w:sz w:val="24"/><w:szCs w:val="24"/></w:rPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Heading5"><w:name w:val="heading 5"/><w:basedOn w:val="Normal"/>` +
	`<w:pPr><w:keepNext/><w:outlineLvl w:val="4"/><w:spacing w:before="160" w:after="60"/></w:pPr>` +
	`<w:rPr><w:b/><w:sz w:val="22"/><w:szCs w:val="22"/></w:rPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Heading6"><w:name w:val="heading 6"/><w:basedOn w:val="Normal"/>` +
	`<w:pPr><w:keepNext/><w:outlineLvl w:val="5"/><w:spacing w:before="160" w:after="60"/></w:pPr>` +
	`<w:rPr><w:b/><w:color w:val="555555"/><w:sz w:val="22"/><w:szCs w:val="22"/></w:rPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Quote"><w:name w:val="Quote"/><w:basedOn w:val="Normal"/>` +
	`<w:pPr><w:ind w:left="480"/></w:pPr><w:rPr><w:i/><w:color w:val="5A5A5A"/></w:rPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Code"><w:name w:val="Code"/><w:basedOn w:val="Normal"/>` +
	`<w:pPr><w:shd w:val="clear" w:fill="F6F8FA"/><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr>` +
	`<w:rPr><w:rFonts w:ascii="Consolas" w:hAnsi="Consolas" w:cs="Consolas"/><w:sz w:val="20"/><w:szCs w:val="20"/></w:rPr></w:style>` +
	`</w:styles>`

// errNoDocument is what reading a zip that is not a Word document reports.
var errNoDocument = errors.New("不是有效的 Word 文档（缺少 word/document.xml）")

// writeDocx renders Markdown as a Word document at path.
func writeDocx(path, text string) error {
	var doc markdown.Document
	doc.Parse(text)
	w := &docxWriter{}
	body := w.body(&doc)
	parts := []struct{ name, data string }{
		{"[Content_Types].xml", docxContentTypes},
		{"_rels/.rels", docxRootRels},
		{"word/document.xml", docxDocument(body)},
		{"word/styles.xml", docxStyles},
		{"word/_rels/document.xml.rels", docxDocumentRels(w.rels.String())},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range parts {
		f, err := zw.Create(p.name)
		if err != nil {
			return err
		}
		if _, err := f.Write([]byte(p.data)); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// docxWriter builds the body of the document and the hyperlink relationships its
// links need.
type docxWriter struct {
	rels strings.Builder
	next int
}

// rel records an external hyperlink and returns the id the run refers to.
func (w *docxWriter) rel(target string) string {
	w.next++
	id := "rId" + strconv.Itoa(w.next+1) // rId1 is the styles part.
	w.rels.WriteString(`<Relationship Id="` + id + `" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="` + xmlEscape(target) + `" TargetMode="External"/>`)
	return id
}

// body turns the blocks of a document into the paragraphs, tables and rules of a
// Word body.
func (w *docxWriter) body(doc *markdown.Document) string {
	var b strings.Builder
	for i := range doc.Blocks {
		blk := &doc.Blocks[i]
		switch blk.Kind {
		case markdown.Rule:
			b.WriteString(`<w:p><w:pPr><w:pBdr><w:bottom w:val="single" w:sz="6" w:space="1" w:color="999999"/></w:pBdr></w:pPr></w:p>`)
		case markdown.Heading:
			b.WriteString(`<w:p><w:pPr><w:pStyle w:val="Heading` + strconv.Itoa(min(max(blk.Level, 1), 6)) + `"/></w:pPr>`)
			w.runs(&b, doc, blk)
			b.WriteString(`</w:p>`)
		case markdown.Code:
			w.code(&b, blk.Text)
		case markdown.Table:
			w.table(&b, doc, blk)
		case markdown.Item:
			b.WriteString(`<w:p><w:pPr>`)
			if blk.Quote > 0 {
				b.WriteString(`<w:pStyle w:val="Quote"/>`)
			}
			b.WriteString(`<w:ind w:left="` + strconv.Itoa(360*(blk.Level+1)) + `" w:hanging="360"/></w:pPr>`)
			marker := "•\t"
			if blk.Number > 0 {
				marker = strconv.Itoa(blk.Number) + ".\t"
			}
			b.WriteString(`<w:r><w:t xml:space="preserve">` + xmlEscape(marker) + `</w:t></w:r>`)
			w.runs(&b, doc, blk)
			b.WriteString(`</w:p>`)
		default:
			b.WriteString(`<w:p>`)
			if blk.Quote > 0 {
				b.WriteString(`<w:pPr><w:pStyle w:val="Quote"/></w:pPr>`)
			}
			w.runs(&b, doc, blk)
			b.WriteString(`</w:p>`)
		}
	}
	return b.String()
}

// runs writes the inline spans of a block.
func (w *docxWriter) runs(b *strings.Builder, doc *markdown.Document, blk *markdown.Block) {
	for _, sp := range doc.Spans(blk) {
		if sp.Image != "" || sp.Text == "" {
			continue
		}
		b.WriteString(w.run(sp.Text, sp.Style, sp.Link))
	}
}

// run writes one span as the runs of a paragraph, keeping bold, italic, strike,
// inline code and links.
func (w *docxWriter) run(text string, style markdown.Style, link string) string {
	var rpr strings.Builder
	if style&markdown.Bold != 0 {
		rpr.WriteString(`<w:b/>`)
	}
	if style&markdown.Italic != 0 {
		rpr.WriteString(`<w:i/>`)
	}
	if style&markdown.Strike != 0 {
		rpr.WriteString(`<w:strike/>`)
	}
	if style&markdown.Mono != 0 {
		rpr.WriteString(`<w:rFonts w:ascii="Consolas" w:hAnsi="Consolas" w:cs="Consolas"/><w:shd w:val="clear" w:color="auto" w:fill="F0F0F0"/>`)
	}
	if link != "" {
		rpr.WriteString(`<w:color w:val="0B57D0"/><w:u w:val="single"/>`)
	}
	var b strings.Builder
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			b.WriteString(`<w:r><w:br/></w:r>`)
		}
		if line == "" {
			continue
		}
		b.WriteString(`<w:r>`)
		if rpr.Len() > 0 {
			b.WriteString(`<w:rPr>` + rpr.String() + `</w:rPr>`)
		}
		b.WriteString(`<w:t xml:space="preserve">` + xmlEscape(line) + `</w:t></w:r>`)
	}
	out := b.String()
	if link != "" && out != "" {
		return `<w:hyperlink r:id="` + w.rel(link) + `">` + out + `</w:hyperlink>`
	}
	return out
}

// code writes a fenced block as one shaded paragraph per line.
func (w *docxWriter) code(b *strings.Builder, text string) {
	for _, line := range strings.Split(text, "\n") {
		b.WriteString(`<w:p><w:pPr><w:pStyle w:val="Code"/></w:pPr>`)
		if line != "" {
			b.WriteString(`<w:r><w:t xml:space="preserve">` + xmlEscape(line) + `</w:t></w:r>`)
		}
		b.WriteString(`</w:p>`)
	}
}

// table writes a pipe table as a Word table, its first row bold.
func (w *docxWriter) table(b *strings.Builder, doc *markdown.Document, blk *markdown.Block) {
	cols := blk.Columns
	if cols <= 0 {
		return
	}
	cells := doc.Cells(blk)
	b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/><w:tblBorders>` +
		`<w:top w:val="single" w:sz="4" w:color="999999"/><w:left w:val="single" w:sz="4" w:color="999999"/>` +
		`<w:bottom w:val="single" w:sz="4" w:color="999999"/><w:right w:val="single" w:sz="4" w:color="999999"/>` +
		`<w:insideH w:val="single" w:sz="4" w:color="999999"/><w:insideV w:val="single" w:sz="4" w:color="999999"/>` +
		`</w:tblBorders></w:tblPr>`)
	for i := 0; i < len(cells); i += cols {
		b.WriteString(`<w:tr>`)
		for j := 0; j < cols && i+j < len(cells); j++ {
			b.WriteString(`<w:tc><w:tcPr><w:tcW w:w="0" w:type="auto"/></w:tcPr><w:p>`)
			for _, sp := range doc.CellSpans(cells[i+j]) {
				style := sp.Style
				if i == 0 {
					style |= markdown.Bold
				}
				b.WriteString(w.run(sp.Text, style, sp.Link))
			}
			b.WriteString(`</w:p></w:tc>`)
		}
		b.WriteString(`</w:tr>`)
	}
	b.WriteString(`</w:tbl>`)
}

// docxDocument wraps a body in the document part Word expects.
func docxDocument(body string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><w:body>` +
		body +
		`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/>` +
		`<w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440" w:header="708" w:footer="708" w:gutter="0"/>` +
		`</w:sectPr></w:body></w:document>`
}

// docxDocumentRels is the document part's relationships: its styles and one
// hyperlink per link.
func docxDocumentRels(links string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
		links + `</Relationships>`
}

// xmlEscape quotes text for XML content or an attribute value.
func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// readDocx returns the Markdown of the Word document at path.
func readDocx(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		return docxMarkdown(rc)
	}
	return "", errNoDocument
}

// docxReader turns the document part back into Markdown. It keeps the blocks it
// has read in a list and joins them at the end, so a list, a code block or a
// table comes out as one run.
type docxReader struct {
	blocks []string

	para  strings.Builder
	style string
	level int
	kind  int
	text  bool

	inTable bool
	inCell  bool
	cell    strings.Builder
	row     []string
	rows    [][]string

	code []string
	list []string
}

// docxMarkdown reads a document part and returns Markdown.
func docxMarkdown(r io.Reader) (string, error) {
	dec := xml.NewDecoder(r)
	rd := &docxReader{}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		rd.token(tok)
	}
	return rd.finish(), nil
}

func (rd *docxReader) token(tok xml.Token) {
	switch t := tok.(type) {
	case xml.StartElement:
		switch t.Name.Local {
		case "tbl":
			rd.flushList()
			rd.flushCode()
			rd.inTable = true
			rd.rows = rd.rows[:0]
		case "tr":
			rd.row = rd.row[:0]
		case "tc":
			rd.inCell = true
			rd.cell.Reset()
		case "p":
			if rd.inCell {
				if rd.cell.Len() > 0 {
					rd.cell.WriteString(" ")
				}
			} else {
				rd.para.Reset()
				rd.style, rd.level, rd.kind = "", 0, 0
			}
		case "pStyle":
			rd.style = docxAttr(t, "val")
		case "numPr":
			rd.kind = 1
		case "ilvl":
			if v, err := strconv.Atoi(docxAttr(t, "val")); err == nil {
				rd.level = v
			}
		case "t":
			rd.text = true
		case "br":
			rd.buf().WriteString("\n")
		case "tab":
			rd.buf().WriteString("\t")
		}
	case xml.EndElement:
		switch t.Name.Local {
		case "t":
			rd.text = false
		case "p":
			if !rd.inCell && !rd.inTable {
				rd.emit()
			}
		case "tc":
			rd.row = append(rd.row, strings.TrimSpace(strings.ReplaceAll(rd.cell.String(), "\n", " ")))
			rd.inCell = false
		case "tr":
			if len(rd.row) > 0 {
				rd.rows = append(rd.rows, append([]string(nil), rd.row...))
			}
		case "tbl":
			rd.inTable = false
			rd.flushTable()
		}
	case xml.CharData:
		if rd.text {
			rd.buf().Write([]byte(t))
		}
	}
}

// buf is where the text of the current run goes: the cell of a table or the
// paragraph.
func (rd *docxReader) buf() *strings.Builder {
	if rd.inCell {
		return &rd.cell
	}
	return &rd.para
}

// emit appends the paragraph that just ended to the blocks.
func (rd *docxReader) emit() {
	text := strings.TrimRight(rd.para.String(), " \t")
	rd.para.Reset()
	style, level, kind := rd.style, rd.level, rd.kind
	rd.style, rd.level, rd.kind = "", 0, 0
	if strings.TrimSpace(text) == "" {
		rd.flushList()
		rd.flushCode()
		return
	}
	if strings.EqualFold(style, "Code") {
		rd.flushList()
		rd.code = append(rd.code, text)
		return
	}
	rd.flushCode()
	if lvl := headingLevel(style); lvl > 0 {
		rd.flushList()
		rd.blocks = append(rd.blocks, strings.Repeat("#", lvl)+" "+strings.TrimSpace(text))
		return
	}
	if strings.EqualFold(style, "Quote") {
		rd.flushList()
		rd.blocks = append(rd.blocks, quoteLines(text))
		return
	}
	trimmed := strings.TrimLeft(text, " \t")
	indent := strings.Repeat("  ", level)
	if kind == 1 || strings.HasPrefix(trimmed, "•") {
		rd.list = append(rd.list, indent+"- "+strings.TrimSpace(strings.TrimPrefix(trimmed, "•")))
		return
	}
	if n, rest, ok := leadingNumber(trimmed); ok {
		rd.list = append(rd.list, indent+strconv.Itoa(n)+". "+rest)
		return
	}
	rd.flushList()
	rd.blocks = append(rd.blocks, text)
}

func (rd *docxReader) flushList() {
	if len(rd.list) == 0 {
		return
	}
	rd.blocks = append(rd.blocks, strings.Join(rd.list, "\n"))
	rd.list = rd.list[:0]
}

func (rd *docxReader) flushCode() {
	if len(rd.code) == 0 {
		return
	}
	rd.blocks = append(rd.blocks, "```\n"+strings.Join(rd.code, "\n")+"\n```")
	rd.code = rd.code[:0]
}

func (rd *docxReader) flushTable() {
	if len(rd.rows) == 0 {
		return
	}
	cols := 0
	for _, r := range rd.rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	var b strings.Builder
	for i, r := range rd.rows {
		b.WriteString("|")
		for j := 0; j < cols; j++ {
			cell := ""
			if j < len(r) {
				cell = strings.ReplaceAll(r[j], "|", "\\|")
			}
			b.WriteString(" " + cell + " |")
		}
		b.WriteString("\n")
		if i == 0 {
			b.WriteString("|")
			for j := 0; j < cols; j++ {
				b.WriteString(" --- |")
			}
			b.WriteString("\n")
		}
	}
	rd.blocks = append(rd.blocks, strings.TrimRight(b.String(), "\n"))
	rd.rows = rd.rows[:0]
}

// finish flushes what is open and joins the blocks.
func (rd *docxReader) finish() string {
	rd.flushList()
	rd.flushCode()
	rd.flushTable()
	if len(rd.blocks) == 0 {
		return ""
	}
	return strings.Join(rd.blocks, "\n\n") + "\n"
}

// docxAttr is the value of the attribute named local, or empty.
func docxAttr(e xml.StartElement, local string) string {
	for _, a := range e.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// headingLevel is the level of a "Heading1" style id, or 0.
func headingLevel(style string) int {
	s := strings.ToLower(strings.TrimSpace(style))
	if !strings.HasPrefix(s, "heading") {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(s, "heading")))
	if err != nil || n < 1 || n > 6 {
		return 0
	}
	return n
}

// leadingNumber reads a "12. " marker at the start of s.
func leadingNumber(s string) (int, string, bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 || i >= len(s) || s[i] != '.' {
		return 0, "", false
	}
	if i+1 >= len(s) || (s[i+1] != ' ' && s[i+1] != '\t') {
		return 0, "", false
	}
	n, err := strconv.Atoi(s[:i])
	if err != nil {
		return 0, "", false
	}
	return n, strings.TrimSpace(s[i+1:]), true
}

// quoteLines prefixes every line of text with a block quote marker.
func quoteLines(text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = "> " + l
	}
	return strings.Join(lines, "\n")
}
