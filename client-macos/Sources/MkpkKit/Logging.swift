import Foundation
import os

/// Shared unified logging for the client (app + runtime kit). Messages are emitted
/// only when `verbose` is enabled (toggled from Settings), and marked `.public` so
/// values are readable. Inspect with:
///   log stream --predicate 'subsystem == "ru.eg23.mkpk.client"' --info
///   log show --last 15m --predicate 'subsystem == "ru.eg23.mkpk.client"' --info
/// Secrets (PSK, tokens) are never logged.
public enum MkpkLog {
    public static let subsystem = "ru.eg23.mkpk.client"
    private static let netLogger = Logger(subsystem: subsystem, category: "net")
    private static let ipLogger  = Logger(subsystem: subsystem, category: "ip")

    /// Verbose diagnostics switch. A plain Bool — reads/writes are atomic in
    /// practice and it flips only from the Settings toggle.
    public nonisolated(unsafe) static var verbose = false

    public static func net(_ msg: @autoclosure () -> String) {
        guard verbose else { return }
        let s = msg()
        netLogger.notice("\(s, privacy: .public)")
    }
    public static func ip(_ msg: @autoclosure () -> String) {
        guard verbose else { return }
        let s = msg()
        ipLogger.notice("\(s, privacy: .public)")
    }
}
