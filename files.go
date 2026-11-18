package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// noteExt reports whether name looks like a note the app can open and edit.
func noteExt(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown", ".mdown", ".mkd", ".mdx", ".txt":
		return true
	}
	return false
}

// codeLang is the language of a source file the tree shows and the preview
// highlights, or "" for a name that is not one. It is what lets the app open a
// Java project, a web folder or any other tree of code, not only notes.
func codeLang(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".java":
		return "java"
	case ".kt", ".kts":
		return "kotlin"
	case ".scala":
		return "scala"
	case ".groovy", ".gradle":
		return "groovy"
	case ".xml", ".xsl", ".xslt", ".xsd", ".svg", ".plist", ".csproj", ".xaml", ".pom":
		return "xml"
	case ".html", ".htm", ".xhtml":
		return "html"
	case ".json", ".jsonc", ".json5":
		return "json"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".jsx":
		return "jsx"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".go":
		return "go"
	case ".c", ".h":
		return "c"
	case ".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".rs":
		return "rust"
	case ".swift":
		return "swift"
	case ".php":
		return "php"
	case ".rb":
		return "ruby"
	case ".py", ".pyw":
		return "python"
	case ".dart":
		return "dart"
	case ".css":
		return "css"
	case ".scss", ".sass":
		return "scss"
	case ".less":
		return "less"
	case ".sql":
		return "sql"
	case ".sh", ".bash", ".zsh":
		return "bash"
	case ".yml", ".yaml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".ini", ".cfg", ".conf", ".properties", ".env":
		return "ini"
	case ".m", ".mm":
		return "objectivec"
	case ".pl", ".pm":
		return "perl"
	case ".lua":
		return "lua"
	case ".r":
		return "r"
	case ".vue":
		return "vue"
	}
	return ""
}

// treeFile reports whether the tree lists name: a note the app edits, or a
// source file it shows with its code highlighted.
func treeFile(name string) bool {
	return noteExt(name) || codeLang(name) != ""
}

// skipName is what the tree and the directory watch leave out: dot files and
// dot directories, and the folders build tools fill with noise.
func skipName(name string) bool {
	if name == "" || name[0] == '.' {
		return true
	}
	switch name {
	case "node_modules", "vendor", "target", "dist", "build", "__pycache__":
		return true
	}
	return false
}

// scanDir lists dir as tree rows: folders first, then notes, each group sorted
// by name. It recurses into folders, and budget caps the total rows so a huge
// tree cannot make the window hang; key numbers the rows so a row keeps its
// focus as folders open and close above it.
func scanDir(dir string, depth int, budget, key *int) []*entry {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	rows := make([]*entry, 0, len(items))
	for _, it := range items {
		name := it.Name()
		if skipName(name) {
			continue
		}
		path := filepath.Join(dir, name)
		if it.IsDir() {
			rows = append(rows, &entry{name: name, path: path, dir: true, depth: depth})
		} else if treeFile(name) {
			rows = append(rows, &entry{name: name, path: path, depth: depth})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.dir != b.dir {
			return a.dir
		}
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	})
	out := rows[:0]
	for _, e := range rows {
		if *budget <= 0 {
			break
		}
		*budget--
		*key++
		e.key = *key
		if e.dir {
			e.kids = scanDir(e.path, depth+1, budget, key)
		}
		out = append(out, e)
	}
	return out
}

// restoreOpen reapplies the open flags gathered before a rescan, so a directory
// watch wake does not close the folders the user had open.
func restoreOpen(list []*entry, open map[string]bool) {
	for _, e := range list {
		if e.dir {
			e.open = open[e.path]
			restoreOpen(e.kids, open)
		}
	}
}

// writeFileAtomic writes data to path by way of a sibling temporary file and a
// rename over path, so a reader never sees a half-written note.
func writeFileAtomic(path, data string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.WriteString(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
