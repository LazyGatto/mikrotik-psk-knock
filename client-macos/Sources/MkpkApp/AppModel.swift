import Foundation
import AppKit
import UniformTypeIdentifiers
import MkpkKit

private let kDetailVariantKey = "mkpk.detailVariant"

/// The app's observable state: the imported invites projected into router groups
/// with per-service status, plus the knock/check actions. @MainActor so SwiftUI
/// can bind directly.
@MainActor
final class AppModel: ObservableObject {
    enum ServiceStatus: Equatable {
        case unknown, checking, knocking, open, closed, error
    }

    /// Outcome of a knock/check attempt, recorded in the per-service log.
    enum LogResult: String {
        case open, closed, timeout, error, sent
        var label: String {
            switch self {
            case .open: return "open"
            case .closed: return "closed"
            case .timeout: return "timeout"
            case .error: return "ошибка"
            case .sent: return "отправлено"
            }
        }
    }

    struct LogEntry: Identifiable {
        let id = UUID()
        let time: Date
        let result: LogResult
        let note: String        // "knock+check" / "check" / "knock"
    }

    /// How per-service details are presented (persisted; chosen in Settings).
    enum DetailVariant: String { case inline, screen }

    /// Which screen the popover shows.
    enum Screen: Equatable { case main, detail(String), settings }

    struct ServiceVM: Identifiable {
        let id: String
        let name: String
        let routerAddress: String
        let checkPort: Int      // 0 → check unavailable ("check off")
        var status: ServiceStatus = .unknown
        var openUntil: Date? = nil     // set after a knock opens the port (countdown)
        // Runtime inputs for actions:
        let router: RouterInvite
        let service: ServiceInvite
        let clientID: String
        var canCheck: Bool { checkPort > 0 }
        var addressLabel: String { checkPort > 0 ? "\(routerAddress):\(checkPort)" : "\(routerAddress) · check off" }
    }

    struct RouterGroup: Identifiable {
        let id: String          // clientID + "\n" + router address (invites for
        let clientID: String    // different client_ids never merge into one group)
        let address: String
        var services: [ServiceVM]
        var reachable: Bool? = nil     // derived from any open/checked service (later: health)
        var clockWarn: Bool = false    // reserved for a future clock hint from the router
    }

    /// A client_id and its routers — the outer visual block and the unit of
    /// deletion (an imported invite is atomic: it carries one client_id).
    struct ClientGroup: Identifiable {
        let id: String          // clientID
        let clientID: String
        var routers: [RouterGroup]
    }

    /// The router groups folded by client_id, preserving import order. Derived
    /// from `groups`, so status mutations there flow through to the UI.
    var clientGroups: [ClientGroup] {
        var byClient: [String: [RouterGroup]] = [:]
        var order: [String] = []
        for g in groups {
            if byClient[g.clientID] == nil { order.append(g.clientID) }
            byClient[g.clientID, default: []].append(g)
        }
        return order.map { ClientGroup(id: $0, clientID: $0, routers: byClient[$0] ?? []) }
    }

    @Published var clientID: String = ""            // shown in the header when a single client_id
    @Published var multipleClients: Bool = false    // ≥2 distinct client_ids imported
    @Published var groups: [RouterGroup] = []
    @Published var lastError: String? {
        didSet { if (oldValue == nil) != (lastError == nil) { onContentChanged?() } }
    }
    @Published var now: Date = Date()   // ticks while a service countdown is live

    /// Max popover height before it scrolls — set by the controller from the
    /// screen's visible height, so the panel grows almost full-height first.
    @Published var maxPanelHeight: CGFloat = 640

    /// The client's public egress IP — the address the router will open access
    /// for (best-effort; shown in the footer). nil until fetched / on failure.
    @Published var publicIP: String?

    // Per-service attempt log (newest first), keyed by service id.
    @Published var logs: [String: [LogEntry]] = [:]

    // Navigation / presentation.
    @Published var screen: Screen = .main
    @Published var expandedServiceID: String?   // inline-expanded row (nil = none)
    @Published var detailVariant: DetailVariant =
        (UserDefaults.standard.string(forKey: kDetailVariantKey).flatMap(DetailVariant.init(rawValue:))) ?? .inline {
        didSet { UserDefaults.standard.set(detailVariant.rawValue, forKey: kDetailVariantKey) }
    }

    private var countdownTimer: Timer?

    private static let hhmm: DateFormatter = {
        let f = DateFormatter(); f.dateFormat = "HH:mm"; return f
    }()

    /// Set by the menu-bar controller: pause/resume the outside-click dismissal
    /// while a modal (the open panel, which runs out-of-process) is up, so the
    /// popover doesn't hide when the user clicks inside that dialog.
    var onSuspendDismissal: ((Bool) -> Void)?

    /// Set by the menu-bar controller: keep the popover open (pinned) so the user
    /// can switch to Finder and drag a file in without the panel dismissing.
    var onPinChanged: ((Bool) -> Void)?

    /// Set by the menu-bar controller: refit the panel to its content after the
    /// groups change (import, error banner appearing/disappearing).
    var onContentChanged: (() -> Void)?

    @Published var pinned: Bool = false {
        didSet { onPinChanged?(pinned) }
    }

    private var store: InviteStore?

    var isEmpty: Bool { groups.isEmpty }

    func load() async {
        // Persistent file backend (works unsigned; the signed build can add the
        // Keychain/iCloud backend). Falls back to in-memory if it can't be created.
        let storage: any InviteStorage = (try? FileInviteStorage()) ?? InMemoryInviteStorage()
        store = try? InviteStore(storage: storage)
        await rebuild()
        refreshPublicIP()
    }

    /// Best-effort public-IP lookup (DNS→OpenDNS with HTTP fallbacks), cached in
    /// `publicIP`. Failures are silent — the footer falls back to generic wording.
    func refreshPublicIP() {
        Task { if let ip = await PublicIP.resolve() { publicIP = ip } }
    }

    @discardableResult
    func importBlob(_ blob: String) async -> Bool {
        do {
            _ = try await store?.importBlob(blob)
            lastError = nil
            await rebuild()
            return true
        } catch {
            lastError = "Не удалось импортировать инвайт: неверный формат."
            return false
        }
    }

    func importFile(_ url: URL) async {
        do {
            let text = try String(contentsOf: url, encoding: .utf8)
            _ = await importBlob(text)
        } catch {
            lastError = "Не удалось прочитать файл."
        }
    }

    /// Open a file picker for `.mkpk` invites.
    func openFilePanel() {
        let panel = NSOpenPanel()
        panel.allowsMultipleSelection = true
        panel.canChooseDirectories = false
        var types: [UTType] = [.plainText, .json]
        if let mkpk = UTType(filenameExtension: "mkpk") { types.insert(mkpk, at: 0) }
        panel.allowedContentTypes = types
        NSApp.activate(ignoringOtherApps: true)
        onSuspendDismissal?(true)
        defer { onSuspendDismissal?(false) }
        guard panel.runModal() == .OK else { return }
        let urls = panel.urls
        Task { for url in urls { await importFile(url) } }
    }

    /// Import an invite blob from the clipboard.
    func pasteBlob() {
        guard let s = NSPasteboard.general.string(forType: .string),
              !s.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            lastError = "В буфере обмена нет текста."
            return
        }
        Task { _ = await importBlob(s) }
    }

    /// Remove an entire client_id block — every imported invite carrying it.
    /// An invite is atomic (one client_id, its routers), so this is the natural
    /// unit of deletion.
    func remove(clientID: String) async {
        guard let store else { return }
        for stored in await store.invites {
            if let inv = try? stored.decoded(), inv.clientID == clientID {
                try? await store.remove(id: stored.id)
            }
        }
        await rebuild()
    }

    private func rebuild() async {
        guard let store else { return }
        let pairs = await store.routers()

        // Group by (client_id, router): invites for different client_ids must not
        // merge into one router card even when they share the same address.
        var byKey: [String: RouterGroup] = [:]
        var order: [String] = []
        var seenServiceIDs: Set<String> = []
        var distinctClients: Set<String> = []

        for (stored, router) in pairs {
            let cid = (try? stored.decoded().clientID) ?? ""
            distinctClients.insert(cid)
            let key = cid + "\n" + router.router
            if byKey[key] == nil {
                byKey[key] = RouterGroup(id: key, clientID: cid, address: router.router, services: [])
                order.append(key)
            }
            for svc in router.services {
                let sid = "\(cid)/\(router.router)/\(svc.name)"
                guard seenServiceIDs.insert(sid).inserted else { continue }   // dedup
                let vm = ServiceVM(id: sid, name: svc.name,
                                   routerAddress: router.router, checkPort: svc.checkPort,
                                   router: router, service: svc, clientID: cid)
                byKey[key]?.services.append(vm)
            }
        }

        multipleClients = distinctClients.count > 1
        clientID = distinctClients.count == 1 ? (distinctClients.first ?? "") : ""
        groups = order.compactMap { byKey[$0] }
        onContentChanged?()
    }

    // MARK: actions

    func knock(_ vm: ServiceVM) {
        update(vm.id, .knocking)
        Task {
            do {
                try await Knock.perform(Knock.options(router: vm.router, service: vm.service, clientID: vm.clientID))
                if vm.canCheck {
                    runCheck(vm, note: "knock+check")
                } else {
                    appendLog(vm.id, .sent, "knock")   // no check_port → can't verify
                    update(vm.id, .unknown)
                }
            } catch {
                appendLog(vm.id, .error, "knock")
                update(vm.id, .error)
            }
        }
    }

    func check(_ vm: ServiceVM) { runCheck(vm, note: "check") }

    private func runCheck(_ vm: ServiceVM, note: String) {
        guard vm.canCheck else { return }
        update(vm.id, .checking)
        Task {
            let res = await Check.run(CheckOptions(host: vm.routerAddress, port: vm.checkPort, timeout: 1, attempts: 6, interval: 0.5))
            switch res.status {
            case .open:
                // Arm the countdown from the service's allowed timeout, if known.
                let until = vm.service.allowedTimeout.flatMap(GoDuration.seconds).map { Date().addingTimeInterval($0) }
                update(vm.id, .open, openUntil: until)
                appendLog(vm.id, .open, note)
                ensureCountdownTimer()
            case .closed:
                update(vm.id, .closed, openUntil: nil)
                appendLog(vm.id, .closed, note)
            case .error:
                update(vm.id, .closed, openUntil: nil)
                appendLog(vm.id, .timeout, note)   // refused/timeout — refined by diagnostics later
            }
        }
    }

    private func appendLog(_ id: String, _ result: LogResult, _ note: String) {
        var arr = logs[id] ?? []
        arr.insert(LogEntry(time: Date(), result: result, note: note), at: 0)
        if arr.count > 20 { arr.removeLast(arr.count - 20) }
        logs[id] = arr
    }

    // MARK: navigation + detail helpers

    func service(id: String) -> ServiceVM? {
        for g in groups { if let s = g.services.first(where: { $0.id == id }) { return s } }
        return nil
    }

    /// Chevron tap: expand inline, or navigate to the detail screen.
    func openDetails(_ vm: ServiceVM) {
        switch detailVariant {
        case .inline: expandedServiceID = (expandedServiceID == vm.id) ? nil : vm.id
        case .screen: screen = .detail(vm.id)
        }
    }

    func showMain() { screen = .main }
    func showSettings() { screen = .settings }

    func lastKnockText(for id: String) -> String {
        guard let d = logs[id]?.first?.time else { return "—" }
        return Self.hhmm.string(from: d)
    }

    func timeLabel(_ d: Date) -> String { Self.hhmm.string(from: d) }

    /// Friendly TTL from the service's allowed_timeout (e.g. "8 мин", "1 ч 30 мин").
    func ttlText(_ vm: ServiceVM) -> String {
        guard let s = vm.service.allowedTimeout, let secs = GoDuration.seconds(s), secs > 0 else { return "—" }
        let total = Int(secs.rounded())
        let h = total / 3600, m = (total % 3600) / 60, sec = total % 60
        if h > 0 { return m > 0 ? "\(h) ч \(m) мин" : "\(h) ч" }
        if m > 0 { return sec > 0 ? "\(m) мин \(sec) с" : "\(m) мин" }
        return "\(sec) с"
    }

    private func update(_ id: String, _ status: ServiceStatus, openUntil: Date?? = nil) {
        for gi in groups.indices {
            if let si = groups[gi].services.firstIndex(where: { $0.id == id }) {
                groups[gi].services[si].status = status
                if let openUntil { groups[gi].services[si].openUntil = openUntil }
                return
            }
        }
    }

    // MARK: countdown

    private var anyOpenWithCountdown: Bool {
        groups.contains { $0.services.contains { $0.status == .open && $0.openUntil != nil } }
    }

    private func ensureCountdownTimer() {
        if anyOpenWithCountdown, countdownTimer == nil {
            let t = Timer(timeInterval: 1, repeats: true) { [weak self] _ in
                Task { @MainActor in self?.tickCountdowns() }
            }
            RunLoop.main.add(t, forMode: .common)
            countdownTimer = t
        } else if !anyOpenWithCountdown {
            countdownTimer?.invalidate()
            countdownTimer = nil
        }
    }

    private func tickCountdowns() {
        now = Date()
        for gi in groups.indices {
            for si in groups[gi].services.indices where groups[gi].services[si].status == .open {
                if let until = groups[gi].services[si].openUntil, until <= now {
                    groups[gi].services[si].status = .closed
                    groups[gi].services[si].openUntil = nil
                }
            }
        }
        ensureCountdownTimer()
    }
}
