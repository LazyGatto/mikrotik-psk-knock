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
	"context"
	"log"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

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

	srv := desktopui.New(store, token, desktopui.KnockTimings{})

	// The wails context arrives at startup; the tray and the /api/open hook
	// need it (guarded — both tolerate a not-yet-ready shell).
	var mu sync.Mutex
	var wailsCtx context.Context
	getCtx := func() context.Context {
		mu.Lock()
		defer mu.Unlock()
		return wailsCtx
	}
	srv.SetOpenURL(func(u string) error {
		ctx := getCtx()
		if ctx == nil {
			return nil
		}
		wailsruntime.BrowserOpenURL(ctx, u)
		return nil
	})

	// Tray (Windows): icon + per-service menu; closing the window hides it to
	// the tray (HideWindowOnClose), not to the taskbar.
	go startTray(srv, store, getCtx, version.String())

	err = wails.Run(&options.App{
		Title:             "mkpk " + version.String(),
		Width:             440,
		Height:            620,
		MinWidth:          380,
		MinHeight:         480,
		HideWindowOnClose: true,
		AssetServer: &assetserver.Options{
			Handler: srv.Handler(),
		},
		OnStartup: func(ctx context.Context) {
			mu.Lock()
			wailsCtx = ctx
			mu.Unlock()
		},
	})
	if err != nil {
		log.Fatalf("mkpk-client: %v", err)
	}
}
