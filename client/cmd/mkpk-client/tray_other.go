//go:build !windows

package main

import (
	"context"

	"mikrotik-psk-knock/client/internal/desktopui"
)

// startTray is a no-op off Windows: the shipped mkpk-client is Windows-first
// (the macOS recipient app is the native client-macos/), and fyne systray
// would demand the main thread on darwin, which wails owns.
func startTray(*desktopui.Server, *desktopui.Store, func() context.Context, func(), string) {}
