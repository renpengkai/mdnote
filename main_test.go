package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"
)

// open runs the page with no window: the real painter, input and focus rules.
func open(t *testing.T, m *model) *app.Driver {
	t.Helper()
	page := ui.NewPage(ui.PageConfig{View: m.page, Theme: &m.theme})
	driver, err := app.NewDriver(page.Config(config(m)), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(driver.Close)
	return driver
}

// click presses the middle of the control with the given name, as a user would.
func click(t *testing.T, driver *app.Driver, name string) {
	t.Helper()
	for _, hit := range driver.Snapshot().Hits {
		if hit.Label == name && !hit.IsLabel {
			b := hit.Bounds
			if err := driver.Click(b.X+b.Width/2, b.Y+b.Height/2); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("no control named %q on screen", name)
}

// enabled reports whether the control with the given name is on screen and not
// disabled.
func enabled(driver *app.Driver, name string) bool {
	for _, hit := range driver.Snapshot().Hits {
		if hit.Label == name && !hit.IsLabel {
			return !hit.Disabled
		}
	}
	return false
}

func shows(driver *app.Driver, want string) bool {
	for _, label := range driver.Snapshot().Labels {
		if strings.Contains(label.Value, want) {
			return true
		}
	}
	return false
}

func writeNote(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTreeOpensANote(t *testing.T) {
	dir := t.TempDir()
	alpha := filepath.Join(dir, "alpha.md")
	writeNote(t, alpha, "# Alpha\n\nHello.\n")
	writeNote(t, filepath.Join(dir, "beta.md"), "# Beta\n")

	m := newModel()
	m.setRoot(dir)
	driver := open(t, m)

	click(t, driver, "alpha.md")
	if m.path != alpha {
		t.Fatalf("clicking alpha.md opened %q, want %q", m.path, alpha)
	}
	if m.text != "# Alpha\n\nHello.\n" {
		t.Fatalf("opened text is %q", m.text)
	}
	if !shows(driver, "字符") {
		t.Fatal("the status bar does not show the note's statistics")
	}
}

func TestSaveWritesTheNote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	writeNote(t, path, "old\n")

	m := newModel()
	m.setRoot(dir)
	driver := open(t, m)
	click(t, driver, "note.md")

	if enabled(driver, "保存") {
		t.Fatal("保存 is enabled on a note with no changes")
	}
	m.text = "new body\n"
	m.invalidator.Invalidate()
	if err := driver.Pump(); err != nil {
		t.Fatal(err)
	}
	if !enabled(driver, "保存") {
		t.Fatal("保存 is disabled after the note changed")
	}
	handled, err := driver.ShortcutChord(ui.Primary, 's')
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("the Primary+S shortcut did not reach the application")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new body\n" {
		t.Fatalf("the file holds %q after save", got)
	}
	if m.saved != m.text {
		t.Fatal("the model still reports unsaved work")
	}
}

func TestThemeToggle(t *testing.T) {
	m := newModel()
	driver := open(t, m)
	if !shows(driver, "深色") {
		t.Fatal("the toolbar does not offer the dark theme")
	}
	click(t, driver, "深色")
	if !m.dark {
		t.Fatal("clicking 深色 did not switch the model to the dark theme")
	}
	if !shows(driver, "浅色") {
		t.Fatal("the toolbar does not offer the light theme after the switch")
	}
}

func TestNewNoteModal(t *testing.T) {
	dir := t.TempDir()
	m := newModel()
	m.setRoot(dir)
	driver := open(t, m)

	click(t, driver, "新建笔记")
	click(t, driver, "创建")
	created := filepath.Join(dir, "新笔记.md")
	if _, err := os.Stat(created); err != nil {
		t.Fatalf("the new note was not created: %v", err)
	}
	if m.path != created {
		t.Fatalf("the new note is not open: %q", m.path)
	}
}

func TestScanDirFiltersAndOrders(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, filepath.Join(dir, "b.md"), "b")
	writeNote(t, filepath.Join(dir, "a.md"), "a")
	writeNote(t, filepath.Join(dir, "note.txt"), "t")
	writeNote(t, filepath.Join(dir, "image.png"), "x")
	writeNote(t, filepath.Join(dir, "sub", "c.md"), "c")
	writeNote(t, filepath.Join(dir, ".hidden", "secret.md"), "s")

	key := 0
	rows := scanDir(dir, 0, &key)

	var names []string
	for _, e := range rows {
		names = append(names, e.name)
	}
	want := []string{"sub", "a.md", "b.md", "note.txt"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("scanDir listed %v, want %v", names, want)
	}
	if rows[0].scanned || rows[0].kids != nil {
		t.Fatal("scanDir must not load folder children until the folder opens")
	}
	loadKids(rows[0], &key)
	if len(rows[0].kids) != 1 || rows[0].kids[0].name != "c.md" {
		t.Fatalf("the subfolder holds %v", rows[0].kids)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	if err := writeFileAtomic(path, "first\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, "second\n"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second\n" {
		t.Fatalf("the file holds %q", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("the atomic write left %d entries behind", len(entries))
	}
}
