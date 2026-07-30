import Foundation
import SwiftUI

/// UI language. English by default; Russian is opt-in from Settings.
enum AppLanguage: String, CaseIterable, Identifiable {
    case en, ru
    var id: String { rawValue }
    var title: String { self == .en ? "English" : "Русский" }
}

/// App-wide localization. A tiny inline catalog: each call site passes both
/// variants — `L("Settings", "Настройки")` — so there is no key registry to keep
/// in sync. Views observe `L10n.shared`, so flipping the language re-renders the
/// whole tree live (no relaunch).
@MainActor
final class L10n: ObservableObject {
    static let shared = L10n()
    private let key = "mkpk.language"

    @Published var language: AppLanguage {
        didSet { UserDefaults.standard.set(language.rawValue, forKey: key) }
    }

    private init() {
        language = UserDefaults.standard.string(forKey: key)
            .flatMap(AppLanguage.init(rawValue:)) ?? .en
    }

    var isRussian: Bool { language == .ru }
}

/// Pick the string for the current language. Main-actor: called from SwiftUI view
/// bodies and the @MainActor app model.
@MainActor func L(_ en: String, _ ru: String) -> String {
    L10n.shared.isRussian ? ru : en
}
