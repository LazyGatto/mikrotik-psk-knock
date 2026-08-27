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
	"sync/atomic"
	"time"

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

	// tucked marks a window we hid ourselves after the user minimised it: the
	// win32 minimised flag survives the hide, so without this the watcher below
	// would fight the tray over showing it again.
	var tucked atomic.Bool
	show := func() {
		ctx := getCtx()
		if ctx == nil {
			return
		}
		tucked.Store(false)
		wailsruntime.WindowUnminimise(ctx)
		wailsruntime.WindowShow(ctx)
	}

	// Tray (Windows): icon + per-service menu.
	go startTray(srv, store, getCtx, show, version.String())

	// Hold-open maintenance: re-knock the services the user asked to keep open,
	// shortly before the router's window runs out. Runs for the life of the app.
	srv.StartKeepOpen(nil)

	err = wails.Run(&options.App{
		Title:     "mkpk " + version.String(),
		Width:     460,
		Height:    640,
		MinWidth:  420,
		MinHeight: 480,
		AssetServer: &assetserver.Options{
			Handler: srv.Handler(),
		},
		OnStartup: func(ctx context.Context) {
			mu.Lock()
			wailsCtx = ctx
			mu.Unlock()
			go watchMinimise(ctx, &tucked)
		},
		// Windows expects the close button to close the program, not to hide it
		// (that is macOS behaviour, and it left people thinking mkpk had quit
		// while it was still knocking from the tray). Ask, then really quit —
		// minimising is what tucks it into the tray.
		OnBeforeClose: func(ctx context.Context) bool {
			return !confirmQuit(ctx, store.Settings().Language)
		},
	})
	if err != nil {
		log.Fatalf("mkpk-client: %v", err)
	}
}

// confirmQuit asks before closing. Wails uses the platform dialog here, which
// is the point: on Windows this must look like every other program's exit
// prompt, not like a web page's confirm box.
func confirmQuit(ctx context.Context, lang string) bool {
	title, msg, yes, no := "Quit mkpk?", "Knocking from the tray stops too.", "Yes", "No"
	if lang == "ru" {
		title, msg = "Выйти из mkpk?", "Стук из трея тоже прекратится."
		yes, no = "Да", "Нет"
	}
	res, err := wailsruntime.MessageDialog(ctx, wailsruntime.MessageDialogOptions{
		Type:          wailsruntime.QuestionDialog,
		Title:         title,
		Message:       msg,
		Buttons:       []string{yes, no},
		DefaultButton: yes,
		CancelButton:  no,
	})
	if err != nil {
		return true // cannot ask → do not trap the user in a window that will not close
	}
	// Windows ignores custom button titles for a question dialog and answers in
	// English; other platforms answer with the label we passed.
	return res == yes || res == "Yes"
}

// watchMinimise turns the minimise button into "tuck into the tray". Wails has
// no minimise callback, so this polls — cheaply, and only to notice a state the
// user just chose by hand.
func watchMinimise(ctx context.Context, tucked *atomic.Bool) {
	tick := time.NewTicker(400 * time.Millisecond)
	defer tick.Stop()
	for range tick.C {
		if tucked.Load() {
			continue
		}
		if wailsruntime.WindowIsMinimised(ctx) {
			tucked.Store(true)
			wailsruntime.WindowHide(ctx)
		}
	}
}
