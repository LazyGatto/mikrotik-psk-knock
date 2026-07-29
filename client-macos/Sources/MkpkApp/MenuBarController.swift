import AppKit
import SwiftUI

/// NSHostingView subclass that reports every SwiftUI relayout, so the panel can
/// refit its height when the content grows or shrinks (import, error banner…).
final class PanelHostingView<Content: View>: NSHostingView<Content> {
    var onLayout: (() -> Void)?
    override func layout() {
        super.layout()
        onLayout?()
    }
}

/// Owns the menu-bar status item and the click-to-open popover panel.
/// Pattern: AppKit NSStatusItem + a borderless non-activating NSPanel hosting
/// SwiftUI, dismissed on outside click (see .agents/skills/macos-menubar-app).
@MainActor
final class MenuBarController: NSObject {
    private let statusItem: NSStatusItem
    private let panel: NSPanel
    private let model: AppModel
    private var outsideClickMonitor: Any?
    private var modalSuspended = false   // an out-of-process modal (open panel) is up
    private var pinned = false           // user pinned the popover (to drag a file in)
    private var isRefitting = false      // re-entrancy guard for the layout-driven refit
    private var maxPanelHeight: CGFloat = 640

    init(model: AppModel) {
        self.model = model
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        panel = NSPanel(contentRect: NSRect(x: 0, y: 0, width: 380, height: 480),
                        styleMask: [.borderless, .nonactivatingPanel],
                        backing: .buffered, defer: false)
        super.init()

        if let button = statusItem.button {
            button.action = #selector(togglePanel)
            button.target = self
        }
        applyMenuState(.idle)

        panel.isFloatingPanel = true
        panel.level = .statusBar
        panel.hidesOnDeactivate = false
        panel.isReleasedWhenClosed = false
        panel.backgroundColor = .clear
        panel.hasShadow = true

        let host = PanelHostingView(rootView: PopoverView(model: model))
        host.autoresizingMask = [.width, .height]
        host.onLayout = { [weak self] in self?.refitIfNeeded() }
        panel.contentView = host
        panel.contentView?.wantsLayer = true
        panel.contentView?.layer?.cornerRadius = 12
        panel.contentView?.layer?.masksToBounds = true

        // Let the model pause outside-click dismissal while a modal (open panel)
        // is up — those run out-of-process and would otherwise hide the popover.
        model.onSuspendDismissal = { [weak self] suspend in
            self?.modalSuspended = suspend
            self?.updateSuspension()
        }
        // Pin keeps the popover open so the user can switch to Finder and drag a
        // file onto the drop zone without the panel dismissing.
        model.onPinChanged = { [weak self] pinned in
            self?.pinned = pinned
            self?.updateSuspension()
        }
        // Refit the panel to its content after the groups/error change.
        model.onContentChanged = { [weak self] in self?.refitIfNeeded() }
        // Reflect the aggregated state in the menu-bar icon.
        model.onMenuStateChanged = { [weak self] state in self?.applyMenuState(state) }
    }

    /// Menu-bar icon by state: neutral shield (idle, template/adaptive), a green
    /// "access open" shield, or an amber attention shield.
    private func applyMenuState(_ state: AppModel.MenuBarState) {
        guard let button = statusItem.button else { return }
        let symbol: String
        let tint: NSColor?
        switch state {
        case .idle:      symbol = "shield.lefthalf.filled"; tint = nil
        case .open:      symbol = "checkmark.shield.fill";  tint = .systemGreen
        case .attention: symbol = "exclamationmark.shield.fill"; tint = .systemOrange
        }
        let image = NSImage(systemSymbolName: symbol, accessibilityDescription: "mkpk")
        if let tint {
            let cfg = NSImage.SymbolConfiguration(paletteColors: [tint])
            let colored = image?.withSymbolConfiguration(cfg)
            colored?.isTemplate = false
            button.image = colored
        } else {
            image?.isTemplate = true   // adapts to the menu-bar appearance
            button.image = image
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

    private var dismissalSuspended: Bool { modalSuspended || pinned }

    private func updateSuspension() {
        if dismissalSuspended {
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
        updateMaxHeight()
        positionPanel()
        panel.makeKeyAndOrderFront(nil)
        if !dismissalSuspended { installMonitor() }
    }

    /// Cap the popover at the screen's visible height (minus small margins) so it
    /// grows nearly full-height before falling back to an internal scroll view.
    private func updateMaxHeight() {
        let screen = statusItem.button?.window?.screen ?? NSScreen.main
        guard let vf = screen?.visibleFrame else { return }
        maxPanelHeight = max(300, vf.height - 24)
        if model.maxPanelHeight != maxPanelHeight { model.maxPanelHeight = maxPanelHeight }
    }

    private func hide() {
        // Never hide while pinned; unpin first to dismiss.
        guard !pinned else { return }
        panel.orderOut(nil)
        removeMonitor()
    }

    /// The content's ideal size, clamped to the panel's width and height bounds.
    private func fittingPanelSize() -> NSSize {
        panel.layoutIfNeeded()
        var size = panel.contentView?.fittingSize ?? NSSize(width: 380, height: 480)
        size.width = 380
        size.height = min(max(size.height, 200), maxPanelHeight)
        return size
    }

    /// Re-run positioning if the content's ideal height no longer matches the
    /// panel. Called from the hosting view's layout pass and on content changes.
    private func refitIfNeeded() {
        guard panel.isVisible, !isRefitting else { return }
        let target = fittingPanelSize()
        guard abs(target.height - panel.frame.height) > 0.5 else { return }
        isRefitting = true
        // Only resize: keep the current on-screen X and pin the TOP edge, so the
        // panel grows/shrinks in place. Recomputing X from the status item here
        // is unsafe — during a layout pass its frame can be momentarily invalid,
        // which flung the panel to the left edge.
        var frame = panel.frame
        let top = frame.maxY
        frame.size = target
        frame.origin.y = top - target.height
        panel.setFrame(frame, display: true)
        isRefitting = false
    }

    private func positionPanel(size preset: NSSize? = nil) {
        guard let button = statusItem.button, let buttonWindow = button.window else { return }
        let buttonFrame = buttonWindow.convertToScreen(button.frame)
        let size = preset ?? fittingPanelSize()
        // Anchor the TOP edge just under the status item so growth extends
        // downward (the panel doesn't jump when content resizes).
        var origin = NSPoint(x: buttonFrame.midX - size.width / 2, y: buttonFrame.minY - size.height - 6)
        let screen = buttonWindow.screen ?? NSScreen.main
        if let vf = screen?.visibleFrame {
            origin.x = min(max(origin.x, vf.minX + 8), vf.maxX - size.width - 8)
        }
        panel.setFrame(NSRect(origin: origin, size: size), display: true)
    }
}
