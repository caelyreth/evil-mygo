package mygo

import (
	"encoding/json/v2"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// The recent documents: those the app opened lately, newest first, which
// RecentDocuments returns and the Open Recent menu lists, kept in
// recent-documents.json in PathUserData. AddRecentDocument records them
// with the system too (the Dock menu on macOS, the jump list on Windows,
// the recently used files on Linux), as NSDocumentController does.
var recentDocs struct {
	sync.Mutex
	loaded bool
	paths  []string
}

// maxRecentDocuments is how many documents the list keeps, as
// NSDocumentController's maximumRecentDocumentCount.
const maxRecentDocuments = 10

func recentFile() (string, error) {
	dir, err := App.Path(PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "recent-documents.json"), nil
}

// loadRecentLocked reads the list once. recentDocs is locked.
func loadRecentLocked() {
	if recentDocs.loaded {
		return
	}
	recentDocs.loaded = true
	if f, err := recentFile(); err == nil {
		if data, err := readAppFile(f); err == nil {
			_ = json.Unmarshal(data, &recentDocs.paths)
		}
	}
}

// AddRecentDocument records path as a document the user opened: it goes
// first in RecentDocuments and the Open Recent menu (RoleOpenRecent), and
// the system offers it again, in the app's Dock menu on macOS, in the
// recently used files of the user (the file chooser's and file manager's
// Recent) on Linux, and in the Recent category of the app's jump list on
// Windows. Documents (NewDocuments) record the files they open and save;
// other apps call it whenever they open a document or folder, including
// one the user chose with a file dialog:
//
//	if err := mygo.App.AddRecentDocument(path); err != nil { … }
//
// A relative path is resolved against the working directory. Choosing an
// entry of the Dock menu, the jump list or the Open Recent menu opens the
// file with the app, which gets it through OnOpenFile, or opens it as a
// document.
func (a *Application) AddRecentDocument(path string) error {
	needsApp("App.AddRecentDocument")
	if path == "" {
		return errors.New("mygo: App.AddRecentDocument needs a path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	return addRecentDocument(abs)
}

// addRecentDocument puts the absolute path first in the recent documents
// and records it with the system.
func addRecentDocument(path string) error {
	recentDocs.Lock()
	loadRecentLocked()
	paths := slices.DeleteFunc(slices.Clone(recentDocs.paths), func(p string) bool { return p == path })
	paths = slices.Insert(paths, 0, path)
	if len(paths) > maxRecentDocuments {
		paths = paths[:maxRecentDocuments]
	}
	recentDocs.paths = paths
	err := saveRecentLocked()
	recentDocs.Unlock()
	return errors.Join(err, onMainValue(func() error { return backend().App().AddRecentDocument(path) }))
}

// ClearRecentDocuments empties RecentDocuments and the Open Recent menu,
// and removes the documents the app recorded with the system: the app's
// recent documents on macOS, the files the app recorded on Linux (the
// user's other recent files stay), and the Recent category of the app's
// jump list on Windows. The Open Recent menu's Clear Menu calls it.
func (a *Application) ClearRecentDocuments() error {
	needsApp("App.ClearRecentDocuments")
	recentDocs.Lock()
	recentDocs.loaded = true
	recentDocs.paths = nil
	err := saveRecentLocked()
	recentDocs.Unlock()
	return errors.Join(err, onMainValue(func() error { return backend().App().ClearRecentDocuments() }))
}

// RecentDocuments returns the documents the app opened lately, newest
// first, which the Open Recent menu (RoleOpenRecent) lists: those of
// AddRecentDocument, also of earlier runs, up to ten. As in AppKit, files
// that no longer exist are left out, and come back if they do again.
func (a *Application) RecentDocuments() []string {
	recentDocs.Lock()
	loadRecentLocked()
	paths := slices.Clone(recentDocs.paths)
	recentDocs.Unlock()
	return slices.DeleteFunc(paths, func(p string) bool {
		_, err := os.Stat(p)
		return errors.Is(err, os.ErrNotExist)
	})
}

// saveRecentLocked writes the recent documents and updates the Open
// Recent menus. recentDocs is locked, so that writes keep the order of
// the changes.
func saveRecentLocked() error {
	postMain(updateRecentMenus)
	f, err := recentFile()
	if err != nil {
		return err
	}
	paths := recentDocs.paths
	return writeAppFile(f, func(w io.Writer) error { return json.MarshalWrite(w, paths) })
}

// openRecentDocument opens a document of the Open Recent menu through
// OnOpenFile, as the system opens those of the Dock menu and the jump list.
// Main thread only.
func openRecentDocument(path string) {
	fire1(&App.onOpenFile, path)
}

// recentMenuItems are the items of the Open Recent submenu.
func recentMenuItems() []*MenuItem {
	var items []*MenuItem
	for _, p := range App.RecentDocuments() {
		items = append(items, &MenuItem{Label: filepath.Base(p), ToolTip: p, Click: func(*MenuItem, *Window) {
			openRecentDocument(p)
		}})
	}
	if len(items) > 0 {
		items = append(items, Separator())
	}
	return append(items, &MenuItem{Label: menuMessages().MenuClearRecent(), Disabled: len(items) == 0, Click: func(*MenuItem, *Window) {
		go func() {
			if err := App.ClearRecentDocuments(); err != nil {
				log.Printf("mygo: recent documents: %v", err)
			}
		}()
	}})
}

// updateRecentMenus gives the Open Recent submenus the recent documents.
// Main thread only.
func updateRecentMenus() {
	if !App.IsReady() {
		return
	}
	changed := false
	menus := []*Menu{App.Menu()}
	for _, w := range Windows() {
		menus = append(menus, w.menu)
	}
	for _, m := range menus {
		walkMenu(m, func(it *MenuItem) {
			if it.Role != RoleOpenRecent {
				return
			}
			items := recentMenuItems()
			menuMu.Lock()
			sub := &Menu{}
			for _, child := range items {
				sub.items = append(sub.items, prepareItem(child, sub))
			}
			it.Submenu, it.sub = items, sub
			menuMu.Unlock()
			changed = true
		})
	}
	if changed {
		refreshMenus()
	}
}

// addRecent records a document's file, as AddRecentDocument does, and logs
// what fails.
func addRecent(path string) {
	if err := addRecentDocument(path); err != nil {
		log.Printf("mygo: documents: %v", err)
	}
}
