// Command mdnote is a small Markdown notes application: a folder of notes on
// the left, an editor and a live preview on the right. It is drawn on the GPU
// by Viva, with no web view, so the program and its window stay small.
package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/components"
	"github.com/vivaui/viva/gfx"
	"github.com/vivaui/viva/markdown"
	"github.com/vivaui/viva/ui"
)

// mode is which panes of a note the window shows.
type mode int

const (
	editOnly mode = iota
	splitView
	previewOnly
)

// dialog is the modal the window is showing, if any.
type dialog int

const (
	noDialog dialog = iota
	newDialog
	mkdirDialog
	renameDialog
	deleteDialog
)

// entry is one row of the file tree: a folder or a note.
type entry struct {
	name    string
	path    string
	dir     bool
	depth   int
	open    bool
	key     int
	kids    []*entry
	scanned bool // dir contents loaded; kids stay nil until the folder opens
}

// model is everything the window shows. page only reads it; a click handled
// where its control is declared is the only place that changes it.
type model struct {
	theme gfx.Theme
	dark  bool

	root    string
	roots   []*entry
	visible []*entry
	tree    ui.TreeState
	sel     string
	split   ui.SplitterState
	mode    mode
	modes   []mode

	path  string
	text  string
	saved string
	lang  string // the code language of a source file, "" for a note

	doc     markdown.Document
	view    components.MarkdownView
	mdText  string
	mdLang  string
	mdWidth float32
	mdH     float32

	statsText  string
	statsCache string

	toaster     ui.Toaster
	invalidator app.Invalidator
	watch       *app.DirectoryWatch
	changes     []string

	menu       bool
	exportOpen bool
	dlg        dialog
	name       string
	target     string

	// Auto-save: the editor arms a timer when the text changes and the timer
	// wakes the window, which writes the note. autoOn turns it off.
	autoOn   bool
	autoCh   chan struct{}
	autoText string
	autoDone bool
	timer    *time.Timer

	treeKey int // monotonic key for tree rows across lazy folder loads

	// Method values made once: a method value made in a paint allocates.
	label   func(int) string
	nodeAt  func(int) ui.TreeNode
	keyAt   func(int) int
	modeOf  func(mode) string
	paint   func(*ui.DrawContext)
	flatten func()
}

func newModel() *model {
	m := &model{
		theme: gfx.LightTheme(),
		// A note opens in the split view: the editor on the left, the rendered
		// preview with its highlighted code on the right. The editor itself is
		// the platform's plain text control, which cannot color code, so the
		// split is what makes the highlighting visible without a click.
		mode:   splitView,
		modes:  []mode{editOnly, splitView, previewOnly},
		split:  ui.SplitterState{Pos: 260},
		autoOn: true,
		autoCh: make(chan struct{}, 1),
	}
	m.label = m.rowLabel
	m.nodeAt = m.rowNode
	m.keyAt = m.rowKey
	m.modeOf = m.modeName
	m.paint = m.paintPreview
	m.flatten = m.rebuild
	m.tree.Node = m.nodeAt
	m.tree.Key = m.keyAt
	return m
}

// page declares the window. It runs again on every repaint, from the model.
func (m *model) page(c ui.C) {
	if ui.Shortcut(c, ui.Primary, "s") {
		m.save()
	}
	if ui.Shortcut(c, ui.Primary, "o") {
		m.openFolder()
	}
	if ui.Shortcut(c, ui.Primary, "n") {
		m.beginNew("")
	}
	m.autoArm()

	ui.Column(c).Fill().Children(func() {
		m.toolbar(c)
		ui.Divider(c)
		ui.Row(c).Fill().Grow(1).Children(func() {
			m.sidebar(c)
			ui.Splitter(c, &m.split).Mins(150, 260)
			m.workspace(c)
		})
		ui.Divider(c)
		m.statusbar(c)
	})
	m.modals(c)
	ui.Toasts(c, &m.toaster)
}

func (m *model) toolbar(c ui.C) {
	ui.Row(c).FillWidth().Pad(8).Gap(8).Items(ui.Center).Children(func() {
		if ui.Button(c, "打开文件夹").Outline().Clicked() {
			m.openFolder()
		}
		if ui.Button(c, "新建笔记").Outline().Clicked() {
			m.beginNew("")
		}
		save := ui.Button(c, "保存").Disabled(m.path == "" || m.text == m.saved)
		if save.Clicked() {
			m.save()
		}
		m.exportMenu(c)
		ui.Spacer(c)
		m.moreMenu(c)
		if ui.Button(c, m.themeLabel()).Outline().Clicked() {
			m.toggleTheme()
		}
	})
}

func (m *model) moreMenu(c ui.C) {
	anchor := ui.Button(c, "更多").Outline()
	if anchor.Clicked() {
		m.menu = !m.menu
	}
	ui.Menu(c, anchor, &m.menu, func(menu *ui.MenuItems) {
		if menu.Item("重命名").Disabled(m.sel == "").Clicked() {
			m.beginRename()
		}
		if menu.Item("删除").Destructive().Disabled(m.sel == "").Clicked() {
			m.beginDelete()
		}
		menu.Separator()
		if menu.Item("导入 Word…").Disabled(m.root == "").Clicked() {
			m.importWord()
		}
		menu.Sub("导出为", func(sub *ui.MenuItems) {
			if sub.Item("Word (.docx)").Disabled(m.path == "").Clicked() {
				m.exportAs("docx")
			}
			if sub.Item("HTML (.html)").Disabled(m.path == "").Clicked() {
				m.exportAs("html")
			}
			if sub.Item("纯文本 (.txt)").Disabled(m.path == "").Clicked() {
				m.exportAs("txt")
			}
		})
		menu.Separator()
		if menu.Item("自动保存").Checked(m.autoOn).Clicked() {
			m.autoOn = !m.autoOn
			if m.autoOn {
				m.autoArm()
			}
		}
		menu.Separator()
		if menu.Item("在文件管理器中显示").Disabled(m.sel == "").Clicked() {
			app.RevealPath(m.sel)
		}
		if menu.Item("重新扫描").Disabled(m.root == "").Clicked() {
			m.rescan()
		}
	})
}

// exportMenu is the toolbar's 导出 button: a menu of the formats a note can be
// written as.
func (m *model) exportMenu(c ui.C) {
	anchor := ui.Button(c, "导出").Outline().Disabled(m.path == "")
	if anchor.Clicked() {
		m.exportOpen = !m.exportOpen
	}
	ui.Menu(c, anchor, &m.exportOpen, func(menu *ui.MenuItems) {
		if menu.Item("Word (.docx)").Clicked() {
			m.exportAs("docx")
		}
		if menu.Item("HTML (.html)").Clicked() {
			m.exportAs("html")
		}
		if menu.Item("纯文本 (.txt)").Clicked() {
			m.exportAs("txt")
		}
	})
}

func (m *model) sidebar(c ui.C) {
	ui.Column(c).Width(m.split.Pos).FillHeight().Background(m.theme.Muted).Children(func() {
		if m.root == "" {
			ui.Column(c).Pad(16).Gap(10).Children(func() {
				ui.Text(c, "还没有打开文件夹").Muted()
				if ui.Button(c, "打开文件夹").Clicked() {
					m.openFolder()
				}
			})
			return
		}
		m.sidebarActions(c)
		ui.Divider(c)
		ui.Tree(c, &m.tree, len(m.visible), m.label, nil).Grow(1)
		if i, ok := m.tree.Toggled(); ok {
			e := m.visible[i]
			e.open = !e.open
			if e.open {
				loadKids(e, &m.treeKey)
			}
			c.After(m.flatten)
		}
		if i, ok := m.tree.Chosen(); ok {
			m.choose(i)
		}
	})
}

func (m *model) workspace(c ui.C) {
	pane := c.Bounds().Width - m.split.Pos - 8
	if pane < 0 {
		pane = 0
	}
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		if m.path == "" {
			ui.Column(c).Fill().Pad(24).Gap(10).Items(ui.Center).Justify(ui.Center).Children(func() {
				ui.Text(c, "选择左侧的笔记，或新建一篇").Muted()
			})
			return
		}
		if m.lang != "" {
			// A source file is read-only: the whole pane is its highlighted code.
			ui.Column(c).Grow(1).FillHeight().Background(m.theme.Card).Children(func() {
				if m.lang == "json" {
					m.jsonBar(c)
				}
				m.preview(c, pane)
			})
			return
		}
		switch m.mode {
		case editOnly:
			m.editor(c)
		case previewOnly:
			m.preview(c, pane)
		default:
			half := (pane - 1) / 2
			ui.Row(c).Fill().Grow(1).Children(func() {
				ui.Column(c).Width(half).FillHeight().Children(func() { m.editor(c) })
				ui.Divider(c)
				ui.Column(c).Grow(1).FillHeight().Background(m.theme.Card).Children(func() {
					m.preview(c, pane-1-half)
				})
			})
		}
		m.modeSwitcher(c)
	})
}

// sidebarActions is the small row above the tree: 新建文件 and 新建文件夹 act on
// the selected folder, or on the root when nothing is selected.
func (m *model) sidebarActions(c ui.C) {
	ui.Row(c).FillWidth().Pad(8).Gap(6).Items(ui.Center).Children(func() {
		if ui.Button(c, "新建文件").Outline().Small().Clicked() {
			m.beginNew(m.folderOf(m.sel))
		}
		if ui.Button(c, "新建文件夹").Outline().Small().Clicked() {
			m.beginNewFolder(m.folderOf(m.sel))
		}
	})
}

// modeSwitcher is the 编辑 / 分栏 / 预览 control. It floats, small, in the bottom
// right corner of the workspace, so it stays out of the note's way.
func (m *model) modeSwitcher(c ui.C) {
	ui.Choice(c, &m.mode, m.modes, m.modeOf).Absolute().Bottom(10).Right(10).FontSize(12)
}

func (m *model) editor(c ui.C) {
	ui.TextArea(c, &m.text, "Markdown 源码", "在这里写 Markdown…").Grow(1).FillWidth()
}

// jsonBar is the strip above a JSON preview: 格式化 pretty-prints the file.
func (m *model) jsonBar(c ui.C) {
	ui.Row(c).FillWidth().Pad(8).Gap(8).Items(ui.Center).Children(func() {
		ui.Text(c, filepath.Base(m.path)).Muted().Ellipsis()
		ui.Spacer(c)
		if ui.Button(c, "格式化").Outline().Small().Clicked() {
			m.formatJSON()
		}
	})
	ui.Divider(c)
}

// formatJSON pretty-prints the open JSON file in place. Save writes it back.
func (m *model) formatJSON() {
	formatted, err := prettyJSON(m.text)
	if err != nil {
		m.toaster.Show("格式化失败", err.Error(), ui.ToastError)
		return
	}
	if formatted == m.text {
		m.toaster.Show("已经是格式化内容", filepath.Base(m.path), ui.ToastInfo)
		return
	}
	m.text = formatted
	m.mdWidth = -1
	m.toaster.Show("已格式化", filepath.Base(m.path), ui.ToastSuccess)
}

// prettyJSON returns text with standard JSON indentation, or an error when the
// text is not valid JSON.
func prettyJSON(text string) (string, error) {
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

func (m *model) preview(c ui.C, width float32) {
	ui.Scroll(c).Grow(1).FillWidth().Pad(16).Scrollbars().Children(func() {
		m.layoutPreview(c.Canvas(), width-32)
		ui.Slot(c).FillWidth().Height(m.mdH).Draw(m.paint)
	})
}

// paintPreview paints the laid-out document into the slot it was declared in.
func (m *model) paintPreview(d *ui.DrawContext) {
	m.view.Paint(d.Canvas, components.MarkdownPaint{
		Look: markdownLook(m.theme, m.dark),
		X:    d.Rect.X,
		Y:    d.Rect.Y,
	})
}

// layoutPreview parses and measures the note, but only when the text or the
// width changed: Layout is the expensive half of the preview.
func (m *model) layoutPreview(canvas *gfx.Canvas, width float32) {
	if width <= 0 || (m.mdText == m.text && m.mdWidth == width && m.mdLang == m.lang) {
		return
	}
	// A source file is shown as one fenced block of its own language, so the
	// same highlighter that colors a note's code colors the whole file.
	source := m.text
	if m.lang != "" {
		source = "```" + m.lang + "\n" + m.text + "\n```"
	}
	m.doc.Parse(source)
	m.mdText, m.mdWidth, m.mdLang = m.text, width, m.lang
	m.mdH = m.view.Layout(canvas, components.MarkdownStyle{
		Size:       m.theme.Metrics.FontSizeM,
		LineHeight: 22,
		CodeInset:  28,
		Highlight:  highlightCode,
	}, &m.doc, width)
}

func (m *model) statusbar(c ui.C) {
	ui.Row(c).FillWidth().Pad(8).Gap(12).Items(ui.Center).Children(func() {
		label := "未打开文件"
		if m.path != "" {
			label = m.path
		}
		ui.Text(c, label).Muted().Ellipsis()
		ui.Spacer(c)
		if m.path != "" && m.text != m.saved {
			ui.Text(c, "未保存").Color(m.theme.Danger)
		} else if m.autoDone {
			ui.Text(c, "已自动保存").Muted()
		}
		ui.Text(c, m.stats()).Muted()
	})
}

func (m *model) modals(c ui.C) {
	switch m.dlg {
	case newDialog:
		open := true
		ui.Modal(c, &open, "新建笔记", func() {
			ui.Column(c).Gap(10).Children(func() {
				ui.Text(c, "新建于 "+m.folderOf(m.target)).Muted()
				ui.TextField(c, &m.name, "文件名", "笔记.md").AutoFocus().FillWidth()
			})
		}, func() {
			if ui.Button(c, "取消").Outline().Clicked() {
				m.dlg = noDialog
			}
			if ui.Button(c, "创建").Clicked() {
				m.createFile()
			}
		})
		if !open {
			m.dlg = noDialog
		}
	case mkdirDialog:
		open := true
		ui.Modal(c, &open, "新建文件夹", func() {
			ui.Column(c).Gap(10).Children(func() {
				ui.Text(c, "新建于 "+m.folderOf(m.target)).Muted()
				ui.TextField(c, &m.name, "文件夹名", "新建文件夹").AutoFocus().FillWidth()
			})
		}, func() {
			if ui.Button(c, "取消").Outline().Clicked() {
				m.dlg = noDialog
			}
			if ui.Button(c, "创建").Clicked() {
				m.createFolder()
			}
		})
		if !open {
			m.dlg = noDialog
		}
	case renameDialog:
		open := true
		ui.Modal(c, &open, "重命名", func() {
			ui.Column(c).Gap(10).Children(func() {
				ui.Text(c, m.target).Muted().Ellipsis()
				ui.TextField(c, &m.name, "新名称", "").AutoFocus().FillWidth()
			})
		}, func() {
			if ui.Button(c, "取消").Outline().Clicked() {
				m.dlg = noDialog
			}
			if ui.Button(c, "重命名").Clicked() {
				m.rename()
			}
		})
		if !open {
			m.dlg = noDialog
		}
	case deleteDialog:
		open := true
		ui.Modal(c, &open, "删除", func() {
			ui.Column(c).Gap(10).Children(func() {
				ui.Text(c, "确定删除 "+m.target+" 吗？此操作无法撤销。")
			})
		}, func() {
			if ui.Button(c, "取消").Outline().Clicked() {
				m.dlg = noDialog
			}
			if ui.Button(c, "删除").Destructive().Clicked() {
				m.deleteTarget()
			}
		})
		if !open {
			m.dlg = noDialog
		}
	}
}

func (m *model) themeLabel() string {
	if m.dark {
		return "浅色"
	}
	return "深色"
}

func (m *model) toggleTheme() {
	m.dark = !m.dark
	if m.dark {
		m.theme = gfx.DarkTheme()
	} else {
		m.theme = gfx.LightTheme()
	}
}

func (m *model) modeName(v mode) string {
	switch v {
	case editOnly:
		return "编辑"
	case previewOnly:
		return "预览"
	default:
		return "分栏"
	}
}

func (m *model) rowLabel(i int) string { return m.visible[i].name }

func (m *model) rowKey(i int) int { return m.visible[i].key }

func (m *model) rowNode(i int) ui.TreeNode {
	e := m.visible[i]
	return ui.TreeNode{
		Depth:    e.depth,
		Branch:   e.dir,
		Expanded: e.open,
		Selected: e.path == m.path,
	}
}

// stats is the character and word count of the note, recomputed only when the
// text changed.
func (m *model) stats() string {
	if m.text == m.statsText {
		return m.statsCache
	}
	words, inWord := 0, false
	for _, r := range m.text {
		switch {
		case unicode.IsSpace(r):
			inWord = false
		case !inWord:
			words, inWord = words+1, true
		}
	}
	m.statsText = m.text
	m.statsCache = strconv.Itoa(utf8.RuneCountInString(m.text)) + " 字符 · " + strconv.Itoa(words) + " 词"
	return m.statsCache
}

func (m *model) choose(i int) {
	e := m.visible[i]
	m.sel = e.path
	if e.dir {
		e.open = !e.open
		if e.open {
			loadKids(e, &m.treeKey)
		}
		m.flatten()
		return
	}
	m.openFile(e.path)
}

func (m *model) folderOf(path string) string {
	if path == "" {
		return m.root
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path
	}
	return filepath.Dir(path)
}

func (m *model) openFolder() {
	dir, err := app.OpenDirectory(app.OpenDirectoryOptions{Title: "选择笔记文件夹"})
	if err != nil || dir == "" {
		return
	}
	m.setRoot(dir)
}

// ensureFolder opens the folder chooser when no folder is open yet, so a new
// note or folder has a home. It reports whether a folder is open afterwards.
func (m *model) ensureFolder() bool {
	if m.root != "" {
		return true
	}
	dir, err := app.OpenDirectory(app.OpenDirectoryOptions{Title: "选择笔记文件夹"})
	if err != nil || dir == "" {
		return false
	}
	m.setRoot(dir)
	return true
}

func (m *model) setRoot(dir string) {
	m.root = dir
	m.sel = ""
	m.rescan()
	m.watchClose()
	if watch, err := app.WatchTree(dir, skipName, m.invalidator.Invalidate); err == nil {
		m.watch = watch
	}
}

func (m *model) watchClose() {
	if m.watch != nil {
		m.watch.Close()
		m.watch = nil
	}
}

// wake applies what the directory watch gathered, on the window thread.
func (m *model) wake() {
	select {
	case <-m.autoCh:
		m.autoSave()
	default:
	}
	if m.watch == nil {
		return
	}
	ch := m.watch.Changes(m.changes[:0])
	m.changes = ch.Names
	if len(m.changes) > 0 || ch.Resync {
		m.rescan()
	}
}

func (m *model) rescan() {
	if m.root == "" {
		m.roots, m.visible = nil, nil
		return
	}
	open := make(map[string]bool)
	var collect func(list []*entry)
	collect = func(list []*entry) {
		for _, e := range list {
			if e.dir && e.open {
				open[e.path] = true
			}
			collect(e.kids)
		}
	}
	collect(m.roots)
	m.treeKey = 0
	m.roots = scanDir(m.root, 0, &m.treeKey)
	restoreOpen(m.roots, open)
	ensureOpenKids(m.roots, &m.treeKey)
	m.rebuild()
}

// rebuild flattens the tree into the rows the Tree shows.
func (m *model) rebuild() {
	m.visible = m.visible[:0]
	var walk func(list []*entry)
	walk = func(list []*entry) {
		for _, e := range list {
			m.visible = append(m.visible, e)
			if e.dir && e.open {
				walk(e.kids)
			}
		}
	}
	walk(m.roots)
}

func (m *model) openFile(path string) {
	if path != m.path {
		m.flushAutoSave()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		m.toaster.Show("无法打开", err.Error(), ui.ToastError)
		return
	}
	m.path = path
	m.text = string(data)
	m.saved = m.text
	m.lang = codeLang(path)
	m.mdWidth = -1
}

func (m *model) save() {
	if m.path == "" {
		return
	}
	if err := writeFileAtomic(m.path, m.text); err != nil {
		m.toaster.Show("保存失败", err.Error(), ui.ToastError)
		return
	}
	m.saved = m.text
	m.autoDone = false
	m.toaster.Show("已保存", filepath.Base(m.path), ui.ToastSuccess)
}

func (m *model) beginNew(dir string) {
	if !m.ensureFolder() {
		return
	}
	if dir == "" {
		dir = m.root
	}
	m.target = dir
	m.name = "新笔记.md"
	m.dlg = newDialog
}

func (m *model) createFile() {
	name := strings.TrimSpace(m.name)
	if name == "" {
		return
	}
	if filepath.Ext(name) == "" {
		name += ".md"
	}
	path := filepath.Join(m.target, name)
	m.dlg = noDialog
	if _, err := os.Stat(path); err == nil {
		m.toaster.Show("已存在", name, ui.ToastWarning)
		m.rescan()
		m.openFile(path)
		return
	}
	title := strings.TrimSuffix(name, filepath.Ext(name))
	if err := os.WriteFile(path, []byte("# "+title+"\n"), 0o644); err != nil {
		m.toaster.Show("创建失败", err.Error(), ui.ToastError)
		return
	}
	m.rescan()
	m.openFile(path)
	m.toaster.Show("已创建", name, ui.ToastSuccess)
}

func (m *model) beginNewFolder(dir string) {
	if !m.ensureFolder() {
		return
	}
	if dir == "" {
		dir = m.root
	}
	m.target = dir
	m.name = "新建文件夹"
	m.dlg = mkdirDialog
}

func (m *model) createFolder() {
	name := strings.TrimSpace(m.name)
	if name == "" {
		return
	}
	path := filepath.Join(m.target, name)
	m.dlg = noDialog
	if err := os.MkdirAll(path, 0o755); err != nil {
		m.toaster.Show("创建失败", err.Error(), ui.ToastError)
		return
	}
	m.sel = path
	m.rescan()
	m.toaster.Show("已创建文件夹", name, ui.ToastSuccess)
}

// importWord reads a Word document and writes the Markdown beside it, then opens
// the note.
func (m *model) importWord() {
	paths, err := app.OpenFiles(app.OpenFileOptions{
		Title:   "导入 Word 文档",
		Filters: []app.FileFilter{{Name: "Word 文档", Pattern: "*.docx"}},
	})
	if err != nil || len(paths) == 0 {
		return
	}
	src := paths[0]
	text, err := readDocx(src)
	if err != nil {
		m.toaster.Show("导入失败", err.Error(), ui.ToastError)
		return
	}
	dst := strings.TrimSuffix(src, filepath.Ext(src)) + ".md"
	if err := os.WriteFile(dst, []byte(text), 0o644); err != nil {
		m.toaster.Show("导入失败", err.Error(), ui.ToastError)
		return
	}
	m.rescan()
	m.openFile(dst)
	m.toaster.Show("已导入 Word", filepath.Base(dst), ui.ToastSuccess)
}

// exportAs writes the open note in another format. An empty format asks for a
// Word document, the first the export menu offers.
func (m *model) exportAs(format string) {
	if m.path == "" {
		return
	}
	base := strings.TrimSuffix(filepath.Base(m.path), filepath.Ext(m.path))
	if format == "" {
		format = "docx"
	}
	options := app.SaveFileOptions{Title: "导出", SuggestedName: base + "." + format}
	switch format {
	case "html":
		options.Filters = []app.FileFilter{{Name: "HTML 网页", Pattern: "*.html"}}
	case "txt":
		options.Filters = []app.FileFilter{{Name: "纯文本", Pattern: "*.txt"}}
	default:
		options.Filters = []app.FileFilter{{Name: "Word 文档", Pattern: "*.docx"}}
	}
	path, err := app.SaveFile(options)
	if err != nil || path == "" {
		return
	}
	var werr error
	switch format {
	case "html":
		werr = writeHTML(path, m.text)
	case "txt":
		werr = writeText(path, m.text)
	default:
		werr = writeDocx(path, m.text)
	}
	if werr != nil {
		m.toaster.Show("导出失败", werr.Error(), ui.ToastError)
		return
	}
	m.toaster.Show("已导出", filepath.Base(path), ui.ToastSuccess)
}

// autoDelay is how long the editor waits after the last keystroke before it
// writes the note.
const autoDelay = 1200 * time.Millisecond

// autoArm starts the auto-save timer when the note has changed since it was last
// written and auto-save is on.
func (m *model) autoArm() {
	if !m.autoOn || m.path == "" || m.text == m.saved || m.text == m.autoText {
		return
	}
	m.autoText = m.text
	m.autoDone = false
	if m.timer != nil {
		m.timer.Stop()
	}
	m.timer = time.AfterFunc(autoDelay, func() {
		select {
		case m.autoCh <- struct{}{}:
		default:
		}
		m.invalidator.Invalidate()
	})
}

// autoSave writes the note when the auto-save timer fires, on the window thread.
func (m *model) autoSave() {
	m.timer = nil
	if !m.autoOn || m.path == "" || m.text == m.saved {
		return
	}
	if err := writeFileAtomic(m.path, m.text); err != nil {
		return
	}
	m.saved = m.text
	m.autoDone = true
}

// flushAutoSave writes unsaved work before the window moves to another note.
func (m *model) flushAutoSave() {
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	if !m.autoOn || m.path == "" || m.text == m.saved {
		return
	}
	if err := writeFileAtomic(m.path, m.text); err == nil {
		m.saved = m.text
	}
}

func (m *model) beginRename() {
	if m.sel == "" {
		return
	}
	m.target = m.sel
	m.name = filepath.Base(m.sel)
	m.dlg = renameDialog
}

func (m *model) rename() {
	name := strings.TrimSpace(m.name)
	if name == "" || m.target == "" {
		return
	}
	dst := filepath.Join(filepath.Dir(m.target), name)
	if dst == m.target {
		m.dlg = noDialog
		return
	}
	if err := os.Rename(m.target, dst); err != nil {
		m.toaster.Show("重命名失败", err.Error(), ui.ToastError)
		return
	}
	m.path = replacePrefix(m.path, m.target, dst)
	m.sel = dst
	m.dlg = noDialog
	m.rescan()
	m.toaster.Show("已重命名", name, ui.ToastSuccess)
}

func (m *model) beginDelete() {
	if m.sel == "" {
		return
	}
	m.target = m.sel
	m.dlg = deleteDialog
}

func (m *model) deleteTarget() {
	target := m.target
	if target == "" {
		return
	}
	if err := os.RemoveAll(target); err != nil {
		m.toaster.Show("删除失败", err.Error(), ui.ToastError)
		return
	}
	if m.path == target || strings.HasPrefix(m.path, target+string(filepath.Separator)) {
		m.path, m.text, m.saved = "", "", ""
	}
	m.sel, m.target, m.dlg = "", "", noDialog
	m.rescan()
	m.toaster.Show("已删除", filepath.Base(target), ui.ToastSuccess)
}

// replacePrefix is path with the old prefix swapped for the new one, when path
// is the prefix itself or lies under it.
func replacePrefix(path, old, updated string) string {
	if path == old {
		return updated
	}
	if strings.HasPrefix(path, old+string(filepath.Separator)) {
		return updated + path[len(old):]
	}
	return path
}

func (m *model) close() {
	m.watchClose()
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
}

func config(m *model) app.Config {
	return app.Config{
		Title:       "MDNote",
		Width:       1120,
		Height:      740,
		MinWidth:    640,
		MinHeight:   420,
		Invalidator: &m.invalidator,
		OnKey:       m.split.Key,
		OnWake:      m.wake,
		OnClosed:    m.close,
	}
}

func main() {
	m := newModel()
	if len(os.Args) > 1 {
		if info, err := os.Stat(os.Args[1]); err == nil {
			if info.IsDir() {
				m.setRoot(os.Args[1])
			} else {
				m.setRoot(filepath.Dir(os.Args[1]))
				m.openFile(os.Args[1])
			}
		}
	}
	page := ui.NewPage(ui.PageConfig{View: m.page, Theme: &m.theme})
	if err := app.RunCanvas(page.Config(config(m)), nil); err != nil {
		log.Fatal(err)
	}
}
