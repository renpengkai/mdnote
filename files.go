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
		} else if noteExt(name) {
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
