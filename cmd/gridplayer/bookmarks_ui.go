package main

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"gridplayer-go/internal/bookmarks"
)

func (a *app) openBookmarkManager() {
	if a.bookmarkWin != nil {
		return
	}
	a.status = ""
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:          "Bookmarks",
		Width:          560,
		Height:         440,
		UseContentSize: true,
		Content:        ui.View(a.bookmarkManagerView),
	})
	a.bookmarkWin = w
	w.OnClose(func(*mygo.CloseEvent) { a.bookmarkWin = nil })
}

func (a *app) bookmarkManagerView(c *ui.Context) {
	key := a.bookmarkKey()
	ui.Column(c).Fill().Padding(16).Gap(10).Children(func() {
		if key == "" {
			ui.Text(c, "No active video").FontSize(14)
			return
		}
		if a.bookmarks == nil {
			ui.Text(c, "Bookmarks are unavailable").FontSize(14)
			return
		}
		ui.Text(c, filepath.Base(key)).FontSize(15).Bold()
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "Add Current").Clicked() {
				a.addBookmark()
			}
			if ui.Button(c, "Clear").Clicked() {
				a.clearBookmarks()
			}
			ui.Spacer(c)
			if ui.Button(c, "Done").Clicked() && a.bookmarkWin != nil {
				a.bookmarkWin.Close()
			}
		})
		if a.status != "" {
			ui.Text(c, a.status).FontSize(12).TextColor(ui.RGB(255, 180, 120))
		}
		ui.Divider(c)
		items := a.bookmarks.List(key)
		if len(items) == 0 {
			ui.Text(c, "No bookmarks for this file").FontSize(13)
			return
		}
		ui.Scroll(c).Fill().Children(func() {
			for _, b := range items {
				b := b
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, fmt.Sprintf("%s — %s", b.Name, fmtClock(b.Position))).FontSize(13)
					ui.Spacer(c)
					if ui.Button(c, "Jump").Clicked() {
						a.jumpBookmark(b.Position)
					}
					if ui.Button(c, "Rename").Clicked() {
						a.openBookmarkRename(b)
					}
					if ui.Button(c, "Delete").Clicked() {
						a.deleteBookmark(b.Name)
					}
				})
			}
		})
	})
}

func (a *app) openBookmarkRename(b bookmarks.Bookmark) {
	if a.bookmarkRenameWin != nil {
		return
	}
	a.bookmarkRenameKey = a.bookmarkKey()
	a.bookmarkRenameOld = b.Name
	a.bookmarkRenameInput = b.Name
	a.status = ""
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:          "Rename Bookmark",
		Width:          360,
		Height:         170,
		UseContentSize: true,
		Content:        ui.View(a.bookmarkRenameView),
	})
	a.bookmarkRenameWin = w
	w.OnClose(func(*mygo.CloseEvent) { a.bookmarkRenameWin = nil })
}

func (a *app) bookmarkRenameView(c *ui.Context) {
	ui.Column(c).Fill().Padding(16).Gap(10).Children(func() {
		ui.Text(c, "Rename bookmark").FontSize(14)
		ui.TextInput(c.Key("bookmark-name"), &a.bookmarkRenameInput)
		if a.status != "" {
			ui.Text(c, a.status).FontSize(12).TextColor(ui.RGB(255, 180, 120))
		}
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "Rename").Clicked() {
				a.renameBookmark()
			}
			if ui.Button(c, "Cancel").Clicked() && a.bookmarkRenameWin != nil {
				a.bookmarkRenameWin.Close()
			}
		})
	})
}

func (a *app) renameBookmark() {
	if a.bookmarks == nil {
		a.status = "bookmarks unavailable"
		return
	}
	if err := a.bookmarks.Rename(a.bookmarkRenameKey, a.bookmarkRenameOld, a.bookmarkRenameInput); err != nil {
		a.status = err.Error()
		return
	}
	if err := a.bookmarks.Save(); err != nil {
		a.status = err.Error()
		return
	}
	a.status = ""
	if a.bookmarkRenameWin != nil {
		a.bookmarkRenameWin.Close()
	}
	if a.bookmarkWin != nil {
		a.bookmarkWin.Invalidate()
	}
	a.refreshMenu()
}

func (a *app) deleteBookmark(name string) {
	key := a.bookmarkKey()
	if a.bookmarks == nil || key == "" {
		return
	}
	a.bookmarks.Remove(key, name)
	if err := a.bookmarks.Save(); err != nil {
		a.status = err.Error()
	} else {
		a.status = ""
	}
	a.refreshMenu()
	if a.bookmarkWin != nil {
		a.bookmarkWin.Invalidate()
	}
}
