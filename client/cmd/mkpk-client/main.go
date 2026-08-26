// mkpk-client is the invite-recipient GUI client (the desktop sibling of the
// native macOS menu-bar app): import .mkpk invites, knock, check. Windows-first
// — the Wails Windows backend is cgo-free, so release binaries are plain
// `GOOS=windows go build -tags desktop,production` cross-compiles.
//
// Same architecture as mkpk-provision-desktop: no wails bindings — the embedded
// webview drives an internal http.Handler (internal/desktopui) over Wails'
// in-process transport, gated by a per-session token injected into the page.
package main

import (
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"mikrotik-psk-knock/client/internal/admin"
	"mikrotik-psk-knock/client/internal/desktopui"
	"mikrotik-psk-knock/client/internal/version"
)

func main() {
	token, err := admin.GenerateSecret(24)
	if err != nil {
		log.Fatalf("mkpk-client: %v", err)
	}
	dir, err := desktopui.DefaultDir()
	if err != nil {
		log.Fatalf("mkpk-client: %v", err)
	}
	store, err := desktopui.NewStore(dir)
	if err != nil {
		log.Fatalf("mkpk-client: %v", err)
	}

	err = wails.Run(&options.App{
		Title:     "mkpk " + version.String(),
		Width:     440,
		Height:    620,
		MinWidth:  380,
		MinHeight: 480,
		AssetServer: &assetserver.Options{
			Handler: desktopui.NewHandler(store, token),
		},
	})
	if err != nil {
		log.Fatalf("mkpk-client: %v", err)
	}
}
