# SKILL: macOS menu-bar app (NSStatusItem + NSPanel popover)

How to build a background menu-bar app with a click-to-open popover. Prefer an
AppKit-owned status item + a custom `NSPanel` over SwiftUI's `MenuBarExtra` —
the panel gives full control over size, dismissal, and hosting SwiftUI.

## App shape

- `LSUIElement = true` in Info.plist → no dock icon, background agent.
- An `NSApplicationDelegate` (via `@NSApplicationDelegateAdaptor`) owns the status
  item and the panel; SwiftUI `App` body can be an empty `Settings {}` scene.
- Single-instance guard: on launch, bail if another copy is running (a named lock
  file or a distributed-notification ping).

## Status item

- `let item = NSStatusBar.system.statusItem(withLength: .variableLength)`.
- `item.button?.image` = a **template** image (`isTemplate = true`) so it adapts
  to light/dark menu bars. Swap the image to reflect state (idle / open / attention).
- `item.button?.action` toggles the panel; handle left vs right click if needed.

## Popover panel

- A borderless `NSPanel` (`styleMask: [.nonactivatingPanel, .fullSizeContentView]`,
  `isFloatingPanel = true`, `level = .statusBar`). Non-activating keeps focus out
  of the dock but the panel can still become key for text fields
  (`becomesKeyOnlyIfNeeded`, override `canBecomeKey`).
- Host SwiftUI with `NSHostingView(rootView:)`; size the panel to the content
  (measure the hosting view's `fittingSize` and resize).
- Position under the status item: compute from `item.button?.window?.frame`.
- Dismiss on outside click: a global/local event monitor
  (`NSEvent.addGlobalMonitorForEvents(matching: [.leftMouseDown, .rightMouseDown])`)
  that closes the panel; also close on `Esc` and on resign-key.

## State-driven icon

Keep an app model (observable) with overall state; when it changes, update the
status-item image/tint. For us: idle (neutral), any-service-open (accent),
attention (amber — knock failed / router unreachable / clock skew).
