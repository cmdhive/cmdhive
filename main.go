// Cmdhive — a terminal that runs one AI agent across several accounts and
// passes the conversation to the next account when one runs out of usage.
package main

import (
	"embed"
	"fmt"
	"os"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	store, err := NewStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ait:", err)
		os.Exit(1)
	}
	app := NewApp(store)
	allowForeground()

	opts := &options.App{
		Title:            "Cmdhive",
		Width:            1100,
		Height:           680,
		MinWidth:         420,
		MinHeight:        260,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 12, G: 12, B: 12, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnBeforeClose:    app.beforeClose,
		DragAndDrop:      &options.DragAndDrop{EnableFileDrop: true},
		Bind:             []any{app},
		// A second launch shows the open window instead of starting another
		// Cmdhive that would fight it over the same accounts and chats.
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "ait-c3a7e0f4-5b1d-4e8a-9f20-6d4b1c8e7a35",
			OnSecondInstanceLaunch: app.secondLaunch,
		},
		Windows: &windows.Options{
			Theme:                windows.Dark,
			WindowClassName:      windowClass,
			DisableWindowIcon:    false,
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	}
	if runtime.GOOS == "darwin" {
		// The Mac's own window buttons sit over Cmdhive's tab bar; the Edit menu is
		// what makes copy and paste work in the web view.
		opts.Frameless = false
		opts.Menu = menu.NewMenuFromItems(menu.AppMenu(), menu.EditMenu(), menu.WindowMenu())
		opts.Mac = &mac.Options{
			TitleBar:   mac.TitleBarHiddenInset(),
			Appearance: mac.NSAppearanceNameDarkAqua,
			About:      &mac.AboutInfo{Title: "Cmdhive", Message: "Version " + Version},
		}
	}
	err = wails.Run(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ait:", err)
		os.Exit(1)
	}
}
