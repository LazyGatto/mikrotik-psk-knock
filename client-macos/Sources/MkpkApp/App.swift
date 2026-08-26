import SwiftUI
import Sparkle

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
    private var updaterController: SPUStandardUpdaterController?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory) // background agent, no dock icon
        let model = AppModel()

        // Sparkle in-app updater. Feed + EdDSA public key live in Info.plist
        // (SUFeedURL / SUPublicEDKey), which build_app.sh writes only for
        // release bundles — dev runs (`swift run MkpkApp`) have no bundle keys,
        // so the updater stays off and Settings hides the button.
        if Bundle.main.object(forInfoDictionaryKey: "SUFeedURL") != nil {
            let updater = SPUStandardUpdaterController(
                startingUpdater: true, updaterDelegate: nil, userDriverDelegate: nil)
            updaterController = updater
            model.checkForUpdates = { updater.checkForUpdates(nil) }
        }

        let mb = MenuBarController(model: model)
        menuBar = mb
        Task { await model.load() }
        if ProcessInfo.processInfo.environment["MKPK_AUTO_OPEN"] != nil {
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.6) { mb.openForDev() }
        }
    }
}
