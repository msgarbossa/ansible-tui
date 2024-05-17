package tui

import (
	"github.com/gdamore/tcell/v2"
)

func (tui *TUI) setupKeyboard() {

	// Setup keyboard shortcuts for the main table (lists images, playbooks, inventory)
	tui.tableMain.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'i':
			// return tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone)
			tui.handleInspectTableCell(tui.tableMain.GetSelection())
		case 'v':
			if tui.editParam == "Inventory" {
				tui.handleVerifyTableCell(tui.tableMain.GetSelection())
			} else {
				return event
			}
		}
		switch event.Key() {
		case tcell.KeyEscape:
			return nil // Return `nil` to avoid default Escape behaviour for the primitive.
		}
		return event
	})

	// Setup app level keyboard shortcuts.
	// SetInputCapture takes top-level keyboard events and processes them before they are passed to the focused widget.
	tui.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		// handle escape key
		case tcell.KeyEscape:
			switch tui.app.GetFocus() {
			case tui.listNav:
				// Ctrl-C should be used to exit
				return nil
				// tui.app.Stop()
				// os.Exit(0)
			case tui.tableMain:
				tui.pages.SwitchToPage("main content")
				tui.textMain1.Clear()
				tui.app.SetFocus(tui.listNav)
				tui.app.Sync()
				return nil // Return `nil` to avoid default Escape behaviour for the primitive.
			case tui.textMain1:

				tui.app.SetFocus(tui.listNav)
				return nil // Return `nil` to avoid default handler for the primitive.
			case tui.textDetail1:
				tui.pages.SwitchToPage("main table")
				tui.app.SetFocus(tui.tableMain)
				tui.app.Sync()
				return nil // Return `nil` to avoid default handler for the primitive.
			}

			// if not on main menu / listNav, set focus to listNav.  Otherwise stop/exit.
			// if tui.app.GetFocus() != tui.listNav {
			// 	tui.app.SetFocus(tui.listNav)
			// } else {
			// 	// TODO: modal dialog to confirm exit
			// 	tui.app.Stop()

			// 	os.Exit(0)
			// }
		}

		// case tell.KeyCtrlI:
		// 	switch tui.app.GetFocus() {
		// 	case tui.tableMain1:
		// 		// switch to textDetail1

		// 	}

		return event
	})

}
