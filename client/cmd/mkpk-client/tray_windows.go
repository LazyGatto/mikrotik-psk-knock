//go:build windows

package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"sort"
	"time"

	"fyne.io/systray"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"mikrotik-psk-knock/client/internal/desktopui"
)

//go:embed assets/tray.ico
var trayIcon []byte

//go:embed assets/tray-open.ico
var trayIconOpen []byte

// trayL is the tray's tiny EN/RU dictionary; the language follows the window's
// setting (re-read on every refresh, so a switch applies within a tick).
func trayL(lang, en, ru string) string {
	if lang == "ru" {
		return ru
	}
	return en
}

// startTray runs the system tray beside wails. fyne.io/systray runs its own
// win32 message loop; on Windows it may live in a goroutine (macOS would
// require the main thread — this file is windows-only). A tray failure must
// not take the app down: the window alone is still a working client.
func startTray(srv *desktopui.Server, store *desktopui.Store, getCtx func() context.Context, appVersion string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("mkpk-client: tray disabled: %v", r)
		}
	}()

	systray.Run(func() { trayReady(srv, store, getCtx, appVersion) }, nil)
}

func trayReady(srv *desktopui.Server, store *desktopui.Store, getCtx func() context.Context, appVersion string) {
	systray.SetIcon(trayIcon)
	systray.SetTitle("mkpk")
	systray.SetTooltip("mkpk " + appVersion)

	lang := store.Settings().Language
	mShow := systray.AddMenuItem(trayL(lang, "Open mkpk", "Открыть mkpk"), "")
	systray.AddSeparator()

	// One menu item per knockable service. The invite list is read once at
	// startup; import/remove happens in the window, and a restart refreshes the
	// tray — acceptable for v1 and keeps the menu code simple.
	type row struct {
		item     *systray.MenuItem
		inviteID string
		router   string
		service  string
	}
	var rows []row
	if states, err := srv.Status(); err == nil {
		sort.SliceStable(states, func(i, j int) bool { return states[i].Router < states[j].Router })
		for _, st := range states {
			it := systray.AddMenuItem(fmt.Sprintf("%s · %s", st.Service, st.Router), "")
			rows = append(rows, row{item: it, inviteID: st.InviteID, router: st.Router, service: st.Service})
		}
	} else {
		log.Printf("mkpk-client: tray status: %v", err)
	}
	systray.AddSeparator()
	mAbout := systray.AddMenuItem("mkpk "+appVersion+" — "+trayL(lang, "About", "О программе"), "")
	mQuit := systray.AddMenuItem(trayL(lang, "Quit", "Выход"), "")

	knocking := map[string]bool{}
	refresh := func() {
		lang = store.Settings().Language
		mShow.SetTitle(trayL(lang, "Open mkpk", "Открыть mkpk"))
		mAbout.SetTitle("mkpk " + appVersion + " — " + trayL(lang, "About", "О программе"))
		mQuit.SetTitle(trayL(lang, "Quit", "Выход"))
		states, err := srv.Status()
		if err != nil {
			return
		}
		byKey := map[string]desktopui.ServiceState{}
		anyOpen := false
		openCount := 0
		for _, st := range states {
			byKey[st.InviteID+"/"+st.Router+"/"+st.Service] = st
			if st.Open() {
				anyOpen = true
				openCount++
			}
		}
		for _, r := range rows {
			key := r.inviteID + "/" + r.router + "/" + r.service
			base := fmt.Sprintf("%s · %s", r.service, r.router)
			switch {
			case knocking[key]:
				r.item.SetTitle(base + " — " + trayL(lang, "knocking…", "стучимся…"))
			case byKey[key].Open():
				left := time.Until(byKey[key].OpenUntil).Round(time.Minute)
				if left < time.Minute {
					left = time.Minute
				}
				r.item.SetTitle(base + " — " + trayL(lang, "open", "открыто") + " · " +
					trayL(lang, fmt.Sprintf("%dm left", int(left.Minutes())), fmt.Sprintf("ещё %dм", int(left.Minutes()))))
			default:
				r.item.SetTitle(base)
			}
		}
		if anyOpen {
			systray.SetIcon(trayIconOpen)
			systray.SetTooltip(fmt.Sprintf("mkpk — %s", trayL(lang, fmt.Sprintf("%d open", openCount), fmt.Sprintf("открыто: %d", openCount))))
		} else {
			systray.SetIcon(trayIcon)
			systray.SetTooltip("mkpk " + appVersion)
		}
	}
	refresh()
	srv.SetOnChange(refresh)

	showWindow := func() {
		if ctx := getCtx(); ctx != nil {
			wailsruntime.WindowShow(ctx)
		}
	}

	go func() {
		tick := time.NewTicker(30 * time.Second) // countdown + expiry rollover
		defer tick.Stop()
		for {
			select {
			case <-mShow.ClickedCh:
				showWindow()
			case <-mAbout.ClickedCh:
				if ctx := getCtx(); ctx != nil {
					wailsruntime.BrowserOpenURL(ctx, desktopui.RepoURL)
				}
			case <-mQuit.ClickedCh:
				if ctx := getCtx(); ctx != nil {
					wailsruntime.Quit(ctx)
				}
				systray.Quit()
				return
			case <-tick.C:
				refresh()
			}
		}
	}()

	// Per-service click loops: knock in a goroutine, reflect progress in the title.
	for _, r := range rows {
		r := r
		key := r.inviteID + "/" + r.router + "/" + r.service
		go func() {
			for range r.item.ClickedCh {
				if knocking[key] {
					continue
				}
				knocking[key] = true
				refresh()
				go func() {
					if _, err := srv.Knock(r.inviteID, r.router, r.service); err != nil {
						log.Printf("mkpk-client: tray knock %s: %v", key, err)
					}
					knocking[key] = false
					refresh()
				}()
			}
		}()
	}
}
