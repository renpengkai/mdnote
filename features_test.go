package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vivaui/viva/components"
)

// classAt is the class of the run that covers byte i, or 0 when none does.
func classAt(runs []components.MarkdownCodeRun, i int) components.MarkdownCodeClass {
	for _, r := range runs {
		if i >= r.Start && i < r.End {
			return r.Class
		}
	}
	return 0
}

func TestHighlightJava(t *testing.T) {
	runs := highlightCode("java", "public class A {", nil)
	if got := classAt(runs, 0); got != components.CodeKeyword {
		t.Fatalf("public is class %d, want keyword", got)
	}
	if got := classAt(runs, 7); got != components.CodeKeyword {
		t.Fatalf("class is class %d, want keyword", got)
	}
	runs = highlightCode("java", "  // 注释", nil)
	if got := classAt(runs, 2); got != components.CodeComment {
		t.Fatalf("the comment is class %d, want comment", got)
	}
	runs = highlightCode("java", `String s = "hi";`, nil)
	if got := classAt(runs, 11); got != components.CodeString {
		t.Fatalf("the string is class %d, want string", got)
	}
}

func TestHighlightXML(t *testing.T) {
	runs := highlightCode("xml", `<a href="x">y</a>`, nil)
	if got := classAt(runs, 0); got != components.CodeKeyword {
		t.Fatalf("the tag marker is class %d, want keyword", got)
	}
	if got := classAt(runs, 3); got != components.CodeVariable {
		t.Fatalf("the attribute name is class %d, want variable", got)
	}
	if got := classAt(runs, 8); got != components.CodeString {
		t.Fatalf("the attribute value is class %d, want string", got)
	}
}

func TestHighlightJSON(t *testing.T) {
	runs := highlightCode("json", `{"a": 1}`, nil)
	if got := classAt(runs, 1); got != components.CodeKey {
		t.Fatalf("the key is class %d, want key", got)
	}
	if got := classAt(runs, 6); got != components.CodeNumber {
		t.Fatalf("the number is class %d, want number", got)
	}
}

func TestHighlightLeavesUnknownLanguagesPlain(t *testing.T) {
	if runs := highlightCode("brainfuck", "+++", nil); len(runs) != 0 {
		t.Fatalf("an unknown language produced %d runs", len(runs))
	}
}

func TestCodeLanguage(t *testing.T) {
	cases := map[string]int{
		"java":            langCLike,
		"XML":             langXML,
		"json":            langJSON,
		"json title=Note": langJSON,
		"go":              langCLike,
		"":                langNone,
		"text":            langNone,
	}
	for info, want := range cases {
		if got := codeLanguage(info); got != want {
			t.Fatalf("codeLanguage(%q) = %d, want %d", info, got, want)
		}
	}
}

func TestDocxRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.docx")
	src := "# 标题\n\n正文 粗体 和 代码。\n\n- 一\n- 二\n\n```java\nint x = 1;\n```\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n"
	if err := writeDocx(path, src); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("the export is not a zip: %v", err)
	}
	defer zr.Close()
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml", "word/styles.xml"} {
		if !names[want] {
			t.Fatalf("the export has no %s", want)
		}
	}
	got, err := readDocx(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# 标题", "正文", "粗体", "- 一", "- 二", "int x = 1;", "| A | B |"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the round trip lost %q:\n%s", want, got)
		}
	}
}

func TestReadDocxRejectsOtherZips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not.docx")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	if _, err := z.Create("hello.txt"); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := readDocx(path); err == nil {
		t.Fatal("a zip that is not a Word document was accepted")
	}
}

func TestExportHTML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.html")
	src := "# 标题\n\n正文 **粗** 与 [链接](https://example.com)。\n\n```json\n{\"a\": 1}\n```\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n"
	if err := writeHTML(path, src); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{"<h1>标题</h1>", "<strong>粗</strong>", `<a href="https://example.com">`, "<pre><code>", "<table>", "<th>A</th>"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the HTML export has no %q:\n%s", want, html)
		}
	}
}

func TestExportText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	src := "# 标题\n\n正文 **粗**。\n\n- 一\n\n```\ncode\n```\n"
	if err := writeText(path, src); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "标题") || !strings.Contains(text, "正文 粗。") || !strings.Contains(text, "- 一") {
		t.Fatalf("the text export reads %q", text)
	}
	if strings.Contains(text, "#") || strings.Contains(text, "**") {
		t.Fatalf("the text export kept markup: %q", text)
	}
}

func TestCreateFolder(t *testing.T) {
	dir := t.TempDir()
	m := newModel()
	m.setRoot(dir)
	m.beginNewFolder("")
	if m.dlg != mkdirDialog {
		t.Fatal("beginNewFolder did not open the folder dialog")
	}
	m.name = "资料"
	m.createFolder()
	info, err := os.Stat(filepath.Join(dir, "资料"))
	if err != nil || !info.IsDir() {
		t.Fatalf("the folder was not created: %v", err)
	}
}

func TestNewFolderFromMenu(t *testing.T) {
	dir := t.TempDir()
	m := newModel()
	m.setRoot(dir)
	driver := open(t, m)
	click(t, driver, "更多")
	click(t, driver, "新建文件夹")
	click(t, driver, "创建")
	if info, err := os.Stat(filepath.Join(dir, "新建文件夹")); err != nil || !info.IsDir() {
		t.Fatalf("the folder was not created from the menu: %v", err)
	}
}

func TestAutoSaveWritesTheNote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	writeNote(t, path, "old\n")

	m := newModel()
	m.setRoot(dir)
	driver := open(t, m)
	click(t, driver, "note.md")

	m.text = "auto\n"
	m.autoArm()
	if m.timer == nil {
		t.Fatal("auto-save did not arm a timer")
	}
	m.timer.Stop()
	m.autoCh <- struct{}{}
	m.wake()
	if err := driver.Pump(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "auto\n" {
		t.Fatalf("auto-save wrote %q", got)
	}
	if m.saved != "auto\n" {
		t.Fatal("auto-save did not mark the note saved")
	}
	if !shows(driver, "已自动保存") {
		t.Fatal("the status bar does not report the auto-save")
	}
}

func TestAutoSaveOffKeepsTheNoteDirty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	writeNote(t, path, "old\n")

	m := newModel()
	m.setRoot(dir)
	driver := open(t, m)
	click(t, driver, "note.md")

	m.autoOn = false
	m.text = "changed\n"
	m.autoArm()
	if m.timer != nil {
		t.Fatal("auto-save armed a timer while it was off")
	}
	m.autoCh <- struct{}{}
	m.wake()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old\n" {
		t.Fatalf("auto-save wrote %q while it was off", got)
	}
}

func TestFlushAutoSaveOnSwitch(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.md")
	writeNote(t, first, "one\n")
	writeNote(t, filepath.Join(dir, "b.md"), "two\n")

	m := newModel()
	m.setRoot(dir)
	driver := open(t, m)
	click(t, driver, "a.md")
	m.text = "one changed\n"
	m.openFile(filepath.Join(dir, "b.md"))

	got, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "one changed\n" {
		t.Fatalf("switching notes wrote %q", got)
	}
}

func TestModeSwitcher(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, filepath.Join(dir, "a.md"), "# A\n")
	m := newModel()
	m.setRoot(dir)
	driver := open(t, m)
	click(t, driver, "a.md")
	click(t, driver, "预览")
	if m.mode != previewOnly {
		t.Fatalf("the mode switcher left the mode at %v", m.mode)
	}
	click(t, driver, "分栏")
	if m.mode != splitView {
		t.Fatalf("the mode switcher left the mode at %v", m.mode)
	}
}