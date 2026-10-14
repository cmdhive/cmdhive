//go:build darwin

package main

import "github.com/wailsapp/wails/v2/pkg/runtime"

// A Mac has the Dock instead of the tray icon and its status panel.
type trayPanel struct{}

func stopTray()           {}
func (a *App) startTray() {}

// TrayState is the main page's latest status for the Windows tray panel.
func (a *App) TrayState(state string) {}

func (a *App) trayTooltip() {}

// HideToTray tucks the window into the Dock; agents keep working.
func (a *App) HideToTray() { runtime.WindowMinimise(a.ctx) }

// ShowApp brings the main window back.
func (a *App) ShowApp() {
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
}
