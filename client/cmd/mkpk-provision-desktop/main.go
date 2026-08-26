// mkpk-provision-desktop is the Wails desktop wrapper around the same local admin
// UI that `mkpk-provision serve` exposes (the recipient GUI
// client is cmd/mkpk-client). Instead of running an HTTP listener, it mounts
// the existing internal/web handler directly as the Wails asset server: the
// embedded webview drives it over Wails' in-process transport (origin
// wails.localhost), so the per-session token and Host guard work unchanged and
// no TCP port is opened.
package main

// SaveFileDialog's darwin implementation references UTType, but the Command
// Line Tools linker does not auto-link its framework — link it explicitly.

/*
#cgo darwin LDFLAGS: -framework UniformTypeIdentifiers
*/
import "C"

import (
	"context"
	"fmt"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"mikrotik-psk-knock/client/internal/admin"
	"mikrotik-psk-knock/client/internal/config"
	"mikrotik-psk-knock/client/internal/version"
	"mikrotik-psk-knock/client/internal/web"
)

func main() {
	// One per-session token, exactly like the serve command. It is injected into
	// the page and required on every API call.
	token, err := admin.GenerateSecret(24)
	if err != nil {
		log.Fatalf("mkpk-desktop: %v", err)
	}
	// The wails context arrives at startup; the handler's desktop hooks (native
	// save dialog, system-browser open) need it, so they read this variable.
	var wailsCtx context.Context

	handler := web.EmbeddedHandlerHooks(config.DefaultPath(), token, web.DesktopHooks{
		SaveDialog: func(filename string) (string, error) {
			if wailsCtx == nil {
				return "", fmt.Errorf("desktop shell not ready")
			}
			return wailsruntime.SaveFileDialog(wailsCtx, wailsruntime.SaveDialogOptions{
				Title:           "Save",
				DefaultFilename: filename,
			})
		},
		OpenURL: func(u string) error {
			if wailsCtx == nil {
				return fmt.Errorf("desktop shell not ready")
			}
			wailsruntime.BrowserOpenURL(wailsCtx, u)
			return nil
		},
	})

	err = wails.Run(&options.App{
		Title:     "mkpk-provision " + version.String(),
		Width:     1200,
		Height:    820,
		MinWidth:  960,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			// Assets nil → every request (GET and non-GET) is served by our
			// handler: index, static assets, favicons and the /api/* endpoints.
			Handler: handler,
		},
		OnStartup: func(ctx context.Context) { wailsCtx = ctx },
	})
	if err != nil {
		log.Fatalf("mkpk-desktop: %v", err)
	}
}
