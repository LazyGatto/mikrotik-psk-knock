# Windows exe resources

`winres/` + the committed `rsrc_windows_*.syso` give the exe its icon (the same
brand tile as the macOS app: `client-macos/Sources/MkpkApp/Resources/icon-dark.png`)
and a manifest (per-monitor-v2 DPI). Plain `go build` links the matching syso
automatically — no wails CLI or extra CI step needed.

Regenerate after changing `winres/winres.json` or the icon:

```sh
go install github.com/tc-hib/go-winres@latest
cd client/cmd/mkpk-desktop && go-winres make --in winres/winres.json --arch amd64,arm64
```
