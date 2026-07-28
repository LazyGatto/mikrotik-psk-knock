# SKILL: macOS SwiftPM app

Building and running a macOS app as a SwiftPM package (no `.xcodeproj`).

## Layout

- `Package.swift` (swift-tools 6.0+), `platforms: [.macOS(.v13)]` or newer.
- Targets: a **library** with the logic (`MkpkKit`), an **executable** for the app
  (the menu-bar `App`), and optionally an executable for CLI/self-check. The
  library holds everything testable; the app target is a thin shell.
- Swift 6 language mode + strict concurrency: `swiftSettings: [.swiftLanguageMode(.v6)]`.

## Build / run

- `swift build` — build all targets.
- `swift run <exe>` — build + run an executable (e.g. `mkpk-selfcheck`).
- **No hot reload**: a menu-bar app is a long-lived process — kill the running
  instance, rebuild, relaunch to test a change.

## Testing gotcha (Command Line Tools only)

- `swift test` (XCTest / Swift Testing) needs **full Xcode**; the CLT lack the
  test runtime (`lib_TestingInterop.dylib`), so `swift test` fails on a CLT-only
  machine. Until Xcode is available, put verifiable checks in a framework-free
  executable (`mkpk-selfcheck`) run via `swift run`.

## Building an .app bundle

`swift build` produces a bare executable, not an `.app`. To ship a menu-bar app:
1. `swift build -c release --arch arm64 --arch x86_64` (universal).
2. Assemble `MyApp.app/Contents/{MacOS/<exe>, Info.plist, Resources/...}` by hand
   or with a small script.
3. `Info.plist` must set `LSUIElement = true` (menu-bar / no dock icon),
   `CFBundleIdentifier`, `CFBundleShortVersionString`, `CFBundleVersion`.
4. Then sign + notarize (see `macos-signing-notarization`).
