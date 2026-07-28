import AppKit
import SwiftUI

/// Owns the menu-bar status item and the click-to-open popover panel.
/// Pattern: AppKit NSStatusItem + a borderless non-activating NSPanel hosting
/// SwiftUI, dismissed on outside click (see .agents/skills/macos-menubar-app).
@MainActor
final class MenuBarController: NSObject {
    private let statusItem: NSStatusItem
    private let panel: NSPanel
    private var outsideClickMonitor: Any?
    private var dismissalSuspended = false

    init(model: AppModel) {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        panel = NSPanel(contentRect: NSRect(x: 0, y: 0, width: 380, height: 480),
                        styleMask: [.borderless, .nonactivatingPanel],
                        backing: .buffered, defer: false)
        super.init()

        if let button = statusItem.button {
            let image = NSImage(systemSymbolName: "shield.lefthalf.filled", accessibilityDescription: "mkpk")
            image?.isTemplate = true
            button.image = image
            button.action = #selector(togglePanel)
            button.target = self
        }

        panel.isFloatingPanel = true
        panel.level = .statusBar
        panel.hidesOnDeactivate = false
        panel.isReleasedWhenClosed = false
        panel.backgroundColor = .clear
        panel.hasShadow = true

        let host = NSHostingView(rootView: PopoverView(model: model))
        host.autoresizingMask = [.width, .height]
        panel.contentView = host
        panel.contentView?.wantsLayer = true
        panel.contentView?.layer?.cornerRadius = 12
        panel.contentView?.layer?.masksToBounds = true

        // Let the model pause outside-click dismissal while a modal (open panel)
        // is up — those run out-of-process and would otherwise hide the popover.
        model.onSuspendDismissal = { [weak self] suspend in
            self?.setDismissalSuspended(suspend)
        }
    }

    private func installMonitor() {
        guard outsideClickMonitor == nil else { return }
        outsideClickMonitor = NSEvent.addGlobalMonitorForEvents(matching: [.leftMouseDown, .rightMouseDown]) { [weak self] _ in
            self?.hide()
        }
    }

    private func removeMonitor() {
        if let m = outsideClickMonitor {
            NSEvent.removeMonitor(m)
            outsideClickMonitor = nil
        }
    }

    private func setDismissalSuspended(_ suspended: Bool) {
        dismissalSuspended = suspended
        if suspended {
            removeMonitor()
        } else if panel.isVisible {
            installMonitor()
        }
    }

    @objc private func togglePanel() {
        panel.isVisible ? hide() : show()
    }

    /// Dev helper: open the panel programmatically (for screenshots). Gated by
    /// the MKPK_AUTO_OPEN env var in the delegate.
    func openForDev() { show() }

    private func show() {
        positionPanel()
        panel.makeKeyAndOrderFront(nil)
        if !dismissalSuspended { installMonitor() }
    }

    private func hide() {
        panel.orderOut(nil)
        removeMonitor()
    }

    private func positionPanel() {
        guard let button = statusItem.button, let buttonWindow = button.window else { return }
        let buttonFrame = buttonWindow.convertToScreen(button.frame)
        panel.layoutIfNeeded()
        var size = panel.contentView?.fittingSize ?? NSSize(width: 380, height: 480)
        if size.width < 320 { size.width = 380 }
        size.height = min(max(size.height, 200), 640)
        var origin = NSPoint(x: buttonFrame.midX - size.width / 2, y: buttonFrame.minY - size.height - 6)
        let screen = buttonWindow.screen ?? NSScreen.main
        if let vf = screen?.visibleFrame {
            origin.x = min(max(origin.x, vf.minX + 8), vf.maxX - size.width - 8)
        }
        panel.setFrame(NSRect(origin: origin, size: size), display: true)
    }
}
