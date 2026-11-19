package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strconv"
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

func TestNewFolderFromSidebar(t *testing.T) {
	dir := t.TempDir()
	m := newModel()
	m.setRoot(dir)
	driver := open(t, m)
	click(t, driver, "新建文件夹")
	click(t, driver, "创建")
	if info, err := os.Stat(filepath.Join(dir, "新建文件夹")); err != nil || !info.IsDir() {
		t.Fatalf("the folder was not created from the sidebar: %v", err)
	}
}

func TestNewFileFromSidebar(t *testing.T) {
	dir := t.TempDir()
	m := newModel()
	m.setRoot(dir)
	driver := open(t, m)
	click(t, driver, "新建文件")
	click(t, driver, "创建")
	if _, err := os.Stat(filepath.Join(dir, "新笔记.md")); err != nil {
		t.Fatalf("the file was not created from the sidebar: %v", err)
	}
}

func TestNewNoteWithoutFolderAsksForOne(t *testing.T) {
	dir := t.TempDir()
	m := newModel()
	driver := open(t, m)

	if !enabled(driver, "新建笔记") {
		t.Fatal("新建笔记 is disabled before a folder is open")
	}
	if err := driver.QueueOpenDirectory(dir, nil); err != nil {
		t.Fatal(err)
	}
	click(t, driver, "新建笔记")
	click(t, driver, "创建")

	created := filepath.Join(dir, "新笔记.md")
	if _, err := os.Stat(created); err != nil {
		t.Fatalf("the new note was not created: %v", err)
	}
	if m.root != dir {
		t.Fatalf("the folder is %q, want %q", m.root, dir)
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
	// The timer would send to autoCh and invalidate the window; Paint forces
	// the wake and the repaint without waiting out the delay.
	m.timer.Stop()
	m.autoCh <- struct{}{}
	if err := driver.Paint(); err != nil {
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

func TestCodeLang(t *testing.T) {
	cases := map[string]string{
		"App.java":     "java",
		"Main.kt":      "kotlin",
		"pom.xml":      "xml",
		"package.json": "json",
		"index.html":   "html",
		"app.ts":       "typescript",
		"main.go":      "go",
		"style.css":    "css",
		"notes.md":     "",
		"readme.txt":   "",
		"photo.png":    "",
	}
	for name, want := range cases {
		if got := codeLang(name); got != want {
			t.Errorf("codeLang(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestTreeFile(t *testing.T) {
	for _, name := range []string{"note.md", "readme.txt", "App.java", "pom.xml", "package.json"} {
		if !treeFile(name) {
			t.Errorf("treeFile(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"icon.png", "clip.mp4", "data.bin"} {
		if treeFile(name) {
			t.Errorf("treeFile(%q) = true, want false", name)
		}
	}
}

func TestTreeListsJavaProject(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, filepath.Join(dir, "src", "main", "java", "App.java"), "public class App {}\n")
	writeNote(t, filepath.Join(dir, "pom.xml"), "<project/>\n")
	writeNote(t, filepath.Join(dir, "package.json"), "{}\n")
	writeNote(t, filepath.Join(dir, "logo.png"), "x")

	key := 0
	rows := scanDir(dir, 0, &key)
	var names []string
	for _, e := range rows {
		names = append(names, e.name)
	}
	want := []string{"src", "package.json", "pom.xml"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("scanDir listed %v, want %v", names, want)
	}
	loadKids(rows[0], &key)
	if len(rows[0].kids) != 1 || rows[0].kids[0].name != "main" {
		t.Fatalf("the src folder holds %v", rows[0].kids)
	}
}

func TestOpenJavaFileIsEditable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "App.java")
	writeNote(t, path, "public class App {}\n")

	m := newModel()
	m.setRoot(dir)
	driver := open(t, m)
	click(t, driver, "App.java")

	if m.path != path {
		t.Fatalf("clicking App.java opened %q, want %q", m.path, path)
	}
	if m.lang != "java" {
		t.Fatalf("the opened file has language %q, want java", m.lang)
	}
	// Source files use the same editor + highlighted preview as notes.
	if !enabled(driver, "编辑") {
		t.Fatal("a source file does not offer the 编辑 mode")
	}
	if !enabled(driver, "源码") {
		t.Fatal("a source file does not show the editor")
	}
	m.text = "public class App { int x = 1; }\n"
	m.save()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != m.text {
		t.Fatalf("saving a Java file wrote %q, want %q", got, m.text)
	}
}

func TestTreeKeepsSiblingsWhenFirstFolderIsHuge(t *testing.T) {
	dir := t.TempDir()
	assets := filepath.Join(dir, "assets")
	backend := filepath.Join(dir, "backend")
	if err := os.MkdirAll(filepath.Join(backend, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		writeNote(t, filepath.Join(backend, "nested", "f"+strconv.Itoa(i)+".go"), "package n\n")
	}
	writeNote(t, filepath.Join(dir, "readme.md"), "# hi\n")
	writeNote(t, filepath.Join(dir, "manifest.json"), "{}\n")

	m := newModel()
	m.setRoot(dir)
	var names []string
	for _, e := range m.roots {
		names = append(names, e.name)
	}
	want := []string{"assets", "backend", "manifest.json", "readme.md"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("root listed %v, want %v", names, want)
	}
}

func TestFormatJSON(t *testing.T) {
	got, err := prettyJSON(`{"b":1,"a":[2,3]}`)
	if err != nil {
		t.Fatal(err)
	}
	// encoding/json sorts map keys, so "a" precedes "b".
	want := "{\n  \"a\": [\n    2,\n    3\n  ],\n  \"b\": 1\n}\n"
	if got != want {
		t.Fatalf("prettyJSON = %q, want %q", got, want)
	}
	if _, err := prettyJSON("{"); err == nil {
		t.Fatal("prettyJSON accepted invalid JSON")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")
	writeNote(t, path, `{"z":1,"y":2}`)
	m := newModel()
	m.setRoot(dir)
	driver := open(t, m)
	click(t, driver, "store.json")
	if !enabled(driver, "格式化") {
		t.Fatal("a JSON file has no 格式化 button")
	}
	click(t, driver, "格式化")
	if !strings.Contains(m.text, "\n  \"z\": 1") {
		t.Fatalf("格式化 left the text as %q", m.text)
	}
	if m.text == m.saved {
		t.Fatal("格式化 should mark the file unsaved")
	}
}

func TestLayoutPreviewDefersWidthOnlyChanges(t *testing.T) {
	m := newModel()
	m.text = "hello"
	m.saved = m.text
	m.mdText = m.text
	m.mdLang = ""
	m.mdWidth = 400
	m.mdH = 100

	// A pure width change must not layout immediately (fullscreen storm).
	m.layoutPreview(nil, 480)
	if m.mdWidth != 400 {
		t.Fatalf("width-only change laid out immediately to %v", m.mdWidth)
	}
	if m.layoutTimer == nil {
		t.Fatal("width-only change did not arm a settle timer")
	}
	m.layoutTimer.Stop()
	m.layoutTimer = nil

	// Settle flag is what the timer sets; consuming it is what allows layout.
	m.layoutDue.Store(true)
	if !m.layoutDue.Swap(false) {
		t.Fatal("layoutDue could not be consumed")
	}
	if m.layoutDue.Load() {
		t.Fatal("layoutDue remained set after swap")
	}
}

func TestScheduleRescanCoalesces(t *testing.T) {
	m := newModel()
	m.scheduleRescan()
	m.scheduleRescan()
	if !m.rescanQueued.Load() {
		t.Fatal("scheduleRescan did not queue a rescan")
	}
	if m.rescanTimer == nil {
		t.Fatal("scheduleRescan did not arm a timer")
	}
	m.rescanTimer.Stop()
	m.rescanTimer = nil
}

func TestSidebarHasNoDuplicateOpenFolder(t *testing.T) {
	dir := t.TempDir()
	m := newModel()
	m.setRoot(dir)
	driver := open(t, m)
	// Toolbar keeps 打开文件夹; the tree action row must not repeat it.
	n := 0
	for _, hit := range driver.Snapshot().Hits {
		if hit.Label == "打开文件夹" && !hit.IsLabel {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("打开文件夹 appeared %d times, want 1", n)
	}
}
