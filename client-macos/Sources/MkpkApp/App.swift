import SwiftUI

@main
struct MkpkAppMain: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) var delegate
    var body: some Scene {
        // Menu-bar only: no main window. The delegate owns the status item + panel.
        Settings { EmptyView() }
    }
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    private var menuBar: MenuBarController?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory) // background agent, no dock icon
        let model = AppModel()
        let mb = MenuBarController(model: model)
        menuBar = mb
        Task { await model.load() }
        if ProcessInfo.processInfo.environment["MKPK_AUTO_OPEN"] != nil {
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.6) { mb.openForDev() }
        }
    }
}
