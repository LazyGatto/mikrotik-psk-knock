// mkpk-desktop is the Wails desktop wrapper around the same local admin UI that
// `mkpk-provision serve` exposes. Instead of running an HTTP listener, it mounts
// the existing internal/web handler directly as the Wails asset server: the
// embedded webview drives it over Wails' in-process transport (origin
// wails.localhost), so the per-session token and Host guard work unchanged and
// no TCP port is opened.
package main

import (
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

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
	handler := web.Handler(config.DefaultPath(), token)

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
	})
	if err != nil {
		log.Fatalf("mkpk-desktop: %v", err)
	}
}
