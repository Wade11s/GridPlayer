package main

import (
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) snapshotName() string {
	n := 1
	if a.snapshots != nil {
		n = len(a.snapshots.List()) + 1
	}
	for {
		name := fmt.Sprintf("Snapshot %d", n)
		if a.snapshots == nil {
			return name
		}
		if _, ok := a.snapshots.Get(name); !ok {
			return name
		}
		n++
	}
}

func (a *app) openSnapshotSave() {
	if a.snapshotSaveWin != nil {
		return
	}
	if a.snapshots == nil {
		a.status = "snapshots unavailable"
		return
	}
	a.snapshotSaveName = a.snapshotName()
	a.status = ""
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:          "Save Snapshot",
		Width:          380,
		Height:         170,
		UseContentSize: true,
		Content:        ui.View(a.snapshotSaveView),
	})
	a.snapshotSaveWin = w
	w.OnClose(func(*mygo.CloseEvent) { a.snapshotSaveWin = nil })
}

func (a *app) snapshotSaveView(c *ui.Context) {
	ui.Column(c).Fill().Padding(16).Gap(10).Children(func() {
		ui.Text(c, "Snapshot name").FontSize(14)
		ui.TextInput(c.Key("snapshot-name"), &a.snapshotSaveName)
		if a.status != "" {
			ui.Text(c, a.status).FontSize(12).TextColor(ui.RGB(255, 180, 120))
		}
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "Save").Clicked() {
				a.saveSnapshot()
			}
			if ui.Button(c, "Cancel").Clicked() && a.snapshotSaveWin != nil {
				a.snapshotSaveWin.Close()
			}
		})
	})
}

func (a *app) saveSnapshot() {
	if a.snapshots == nil {
		a.status = "snapshots unavailable"
		return
	}
	if err := a.snapshots.Save(a.snapshotSaveName, a.document()); err != nil {
		a.status = err.Error()
		return
	}
	a.status = ""
	if a.snapshotSaveWin != nil {
		a.snapshotSaveWin.Close()
	}
	a.refreshMenu()
}

func (a *app) restoreSnapshot(name string) {
	if a.snapshots == nil {
		return
	}
	d, ok := a.snapshots.Get(name)
	if !ok {
		a.status = "snapshot not found"
		return
	}
	a.applyDocument(d, "")
	a.dirty = true
	a.status = "restored " + name
}

func (a *app) deleteSnapshot(name string) {
	if a.snapshots == nil {
		return
	}
	if err := a.snapshots.Remove(name); err != nil {
		a.status = err.Error()
		return
	}
	a.status = ""
	a.refreshMenu()
}

func (a *app) snapshotMenuItems() (restore, remove []*mygo.MenuItem) {
	if a.snapshots != nil {
		for _, s := range a.snapshots.List() {
			s := s
			restore = append(restore, &mygo.MenuItem{
				Label: s.Name,
				Click: func(*mygo.MenuItem, *mygo.Window) { a.restoreSnapshot(s.Name) },
			})
			remove = append(remove, &mygo.MenuItem{
				Label: s.Name,
				Click: func(*mygo.MenuItem, *mygo.Window) { a.deleteSnapshot(s.Name) },
			})
		}
	}
	if len(restore) == 0 {
		restore = append(restore, &mygo.MenuItem{Label: "No Snapshots", Disabled: true})
		remove = append(remove, &mygo.MenuItem{Label: "No Snapshots", Disabled: true})
	}
	return restore, remove
}
