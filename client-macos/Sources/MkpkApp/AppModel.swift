import Foundation
import AppKit
import Network
import ServiceManagement
import UserNotifications
import UniformTypeIdentifiers
import MkpkKit

private let kDetailVariantKey = "mkpk.detailVariant"
private let kNotificationsKey = "mkpk.notifications"
private let kICloudSyncKey = "mkpk.iCloudSync"
private let kKeepOpenKey = "mkpk.keepOpen"
private let kVerboseLogKey = "mkpk.verboseLog"

/// The app's observable state: the imported invites projected into router groups
/// with per-service status, plus the knock/check actions. @MainActor so SwiftUI
/// can bind directly.
@MainActor
final class AppModel: ObservableObject {
    enum ServiceStatus: Equatable {
        case unknown, checking, knocking, open, closed, error
        // Knock sent, but reachability can't be confirmed — the check ran through a
        // VPN/transparent-proxy tunnel that answers the TCP handshake locally.
        case unverified
    }

    /// Outcome of a knock/check attempt, recorded in the per-service log.
    enum LogResult: String {
        case open, closed, timeout, error, sent, unverified
    }

    /// Localized label for a log result (called from the log view).
    func logLabel(_ r: LogResult) -> String {
        switch r {
        case .open: return L("open", "открыто")
        case .closed: return L("closed", "закрыто")
        case .timeout: return L("timeout", "таймаут")
        case .error: return L("error", "ошибка")
        case .sent: return L("sent", "отправлено")
        case .unverified: return L("unverified", "не подтв.")
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

    /// Aggregated menu-bar icon state.
    enum MenuBarState: Equatable { case idle, open, attention }

    struct ServiceVM: Identifiable {
        let id: String
        let name: String
        let routerAddress: String
        let checkPort: Int      // 0 → check unavailable ("check off")
        var status: ServiceStatus = .unknown
        var openUntil: Date? = nil     // set after a knock opens the port (countdown)
        var openedForIP: String? = nil // egress IP the port was opened for (stale-IP detection)
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

    // MARK: settings (#5)

    /// Launch at login — truth lives in SMAppService; the stored value mirrors it.
    @Published var launchAtLogin: Bool = (SMAppService.mainApp.status == .enabled) {
        didSet { if !syncingLaunch { applyLaunchAtLogin() } }
    }
    private var syncingLaunch = false

    /// Post a notification when a service opens / auto-closes.
    @Published var notificationsEnabled: Bool = UserDefaults.standard.bool(forKey: kNotificationsKey) {
        didSet {
            UserDefaults.standard.set(notificationsEnabled, forKey: kNotificationsKey)
            if notificationsEnabled { requestNotificationAuthorization() }
        }
    }
    private var notificationsAuthorized = false

    /// Sync imported invites via iCloud Keychain (needs a signed build to really
    /// sync; the toggle swaps the storage backend and migrates existing invites).
    @Published var iCloudSync: Bool = UserDefaults.standard.bool(forKey: kICloudSyncKey) {
        didSet {
            guard !syncingStorage, oldValue != iCloudSync else { return }
            UserDefaults.standard.set(iCloudSync, forKey: kICloudSyncKey)
            Task { await reconfigureStorage() }
        }
    }
    private var syncingStorage = false

    /// Verbose diagnostic logging to the unified log (off by default). When on,
    /// per-attempt check paths/interfaces, IP resolution and network-path changes
    /// are logged under subsystem "ru.eg23.mkpk.client".
    @Published var verboseLogging: Bool = UserDefaults.standard.bool(forKey: kVerboseLogKey) {
        didSet {
            UserDefaults.standard.set(verboseLogging, forKey: kVerboseLogKey)
            MkpkLog.verbose = verboseLogging
        }
    }

    // MARK: keep-open (#1)

    /// Services the user asked to hold open — re-knocked shortly before the
    /// allowed_timeout expires. Persisted by service id.
    @Published var keepOpenIDs: Set<String> =
        Set(UserDefaults.standard.stringArray(forKey: kKeepOpenKey) ?? [])
    private var renewingIDs: Set<String> = []          // renew in flight (no overlap)
    private var renewFailures: [String: Int] = [:]     // consecutive keep-open failures

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

    /// Set by the menu-bar controller: reflect the aggregated state in the icon.
    var onMenuStateChanged: ((MenuBarState) -> Void)?
    private var lastMenuState: MenuBarState = .idle

    /// idle / open (something is open) / attention (a service errored, keep-open
    /// gave up, or a port is open for a now-stale egress IP).
    var menuBarState: MenuBarState {
        var open = false, attention = false
        for g in groups {
            for s in g.services {
                if s.status == .error { attention = true }
                if s.status == .open {
                    open = true
                    if ipStale(s) { attention = true }
                }
            }
        }
        if attention { return .attention }
        return open ? .open : .idle
    }

    /// A service is open but the router opened it for an egress IP that no longer
    /// matches ours (reconnect / switched network) — traffic won't pass until re-knock.
    ///
    /// NOTE: both client-side signals are imperfect — the public-IP probe measures
    /// the general internet path (can differ from the source the router sees under a
    /// split-tunnel VPN), and the TCP check only proves *something* answers on
    /// `router:checkPort` (an unrelated dst-nat/router service can answer). We keep
    /// this IP heuristic as the safe-side warning; `mkpk-provision test` (over SSH)
    /// remains the only authoritative check. Under investigation — see the logs.
    func ipStale(_ vm: ServiceVM) -> Bool {
        guard vm.status == .open, let opened = vm.openedForIP, let cur = publicIP else { return false }
        return opened != cur
    }

    /// Any open service whose opening IP no longer matches — drives the footer warning.
    var hasStaleOpenIP: Bool {
        groups.contains { $0.services.contains { ipStale($0) } }
    }

    private func refreshMenuState() {
        let s = menuBarState
        if s != lastMenuState { lastMenuState = s; onMenuStateChanged?(s) }
    }

    @Published var pinned: Bool = false {
        didSet { onPinChanged?(pinned) }
    }

    private var store: InviteStore?

    var isEmpty: Bool { groups.isEmpty }

    func load() async {
        store = (try? InviteStore(storage: makeStorage()))
             ?? (try? InviteStore(storage: (try? FileInviteStorage()) ?? InMemoryInviteStorage()))
        MkpkLog.verbose = verboseLogging
        await rebuild()
        refreshPublicIP()
        startPathMonitor()
        refreshNotificationAuthorization()
    }

    /// Set (or clear) the egress IP a service was opened for.
    private func setOpenedIP(_ id: String, _ ip: String?) {
        for gi in groups.indices {
            if let si = groups[gi].services.firstIndex(where: { $0.id == id }) {
                groups[gi].services[si].openedForIP = ip; return
            }
        }
    }

    /// The invite storage backend for the current sync preference. File backend
    /// works unsigned; the Keychain backend (synchronizable = iCloud Keychain)
    /// really syncs only in a signed build.
    private func makeStorage() -> any InviteStorage {
        if iCloudSync {
            return KeychainInviteStorage(synchronizable: true)
        }
        return (try? FileInviteStorage()) ?? InMemoryInviteStorage()
    }

    /// Swap the storage backend (on the iCloud toggle) and migrate existing
    /// invites. Reverts the toggle if the new backend can't be written.
    private func reconfigureStorage() async {
        let existing = await store?.invites ?? []
        do {
            let newStore = try InviteStore(storage: makeStorage())
            let have = Set(await newStore.invites.map { $0.id })
            for inv in existing where !have.contains(inv.id) {
                _ = try await newStore.importBlob(inv.blob)
            }
            store = newStore
            lastError = nil
            await rebuild()
        } catch {
            syncingStorage = true
            iCloudSync.toggle()
            UserDefaults.standard.set(iCloudSync, forKey: kICloudSyncKey)
            syncingStorage = false
            lastError = L("iCloud sync isn't available yet (needs a provisioning profile for keychain access).",
                          "iCloud-синхронизация пока недоступна (нужен provisioning profile для keychain-доступа).")
        }
    }

    // MARK: autostart / notifications

    private func applyLaunchAtLogin() {
        let svc = SMAppService.mainApp
        do {
            if launchAtLogin {
                if svc.status != .enabled { try svc.register() }
            } else if svc.status == .enabled {
                try svc.unregister()
            }
        } catch {
            lastError = L("Couldn't change launch-at-login (needs a built .app).",
                          "Не удалось изменить автозапуск (нужна собранная .app).")
            syncingLaunch = true
            launchAtLogin = (svc.status == .enabled)
            syncingLaunch = false
        }
    }

    private func requestNotificationAuthorization() {
        guard Bundle.main.bundleIdentifier != nil else {
            lastError = L("Notifications work only in a built .app.", "Уведомления доступны только в собранной .app.")
            notificationsEnabled = false
            return
        }
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound]) { [weak self] granted, error in
            let message = error?.localizedDescription
            Task { @MainActor in
                guard let self else { return }
                self.notificationsAuthorized = granted
                if !granted {
                    self.notificationsEnabled = false
                    self.lastError = message
                        ?? L("Notifications aren't allowed — enable them in System Settings (or a signed build is needed).",
                              "Уведомления не разрешены — включите их в Системных настройках (или нужна подписанная сборка).")
                }
            }
        }
    }

    private func refreshNotificationAuthorization() {
        guard Bundle.main.bundleIdentifier != nil else { return }
        UNUserNotificationCenter.current().getNotificationSettings { [weak self] s in
            let authorized = (s.authorizationStatus == .authorized)
            Task { @MainActor in self?.notificationsAuthorized = authorized }
        }
    }

    private func notify(_ title: String, _ body: String) {
        guard notificationsEnabled, notificationsAuthorized, Bundle.main.bundleIdentifier != nil else { return }
        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        UNUserNotificationCenter.current().add(
            UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil))
    }

    /// True while a public-IP lookup is in flight (drives the footer spinner and
    /// prevents overlapping resolves).
    @Published var resolvingIP = false

    /// Best-effort public-IP lookup (DNS→OpenDNS with HTTP fallbacks), cached in
    /// `publicIP`. Detects an egress-IP change: open ports were opened for the old
    /// address and won't pass traffic from the new one until re-knocked. Backfills
    /// the opening IP for services that opened before we knew it (so the true,
    /// post-knock egress IP is recorded — not a possibly-stale cache). Failures are
    /// silent — the footer falls back to generic wording.
    func refreshPublicIP() {
        guard !resolvingIP else { return }
        resolvingIP = true
        Task {
            let resolved = await PublicIP.resolve()
            resolvingIP = false
            guard let ip = resolved else { MkpkLog.ip("public IP resolve failed"); return }
            let previous = publicIP
            if previous != ip {
                MkpkLog.ip("public IP: \(previous ?? "—") -> \(ip)")
            }
            publicIP = ip
            for gi in groups.indices {
                for si in groups[gi].services.indices
                where groups[gi].services[si].status == .open && groups[gi].services[si].openedForIP == nil {
                    groups[gi].services[si].openedForIP = ip   // record the current egress IP
                }
            }
            if let previous, previous != ip { handleIPChanged(from: previous, to: ip) }
            refreshMenuState()
        }
    }

    /// Egress IP changed while ports were open — warn once (the visual stale badges
    /// update live off `publicIP`).
    private func handleIPChanged(from old: String, to new: String) {
        let stale = groups.flatMap { $0.services }.filter {
            $0.status == .open && ($0.openedForIP ?? new) != new
        }
        MkpkLog.ip("IP changed \(old)->\(new); open-but-stale by IP: \(stale.map(\.name).joined(separator: ", "))")
        guard !stale.isEmpty else { return }
        let names = stale.map { $0.name }.joined(separator: ", ")
        notify(L("External IP changed", "Внешний IP изменился"),
               L("Open for \(old) — re-knock for \(new): \(names)", "Открыто для \(old) — перестучите для \(new): \(names)"))
    }

    // Re-resolve the egress IP when the network path changes (reconnect, Wi-Fi↔
    // cellular) — the most direct signal that the address may have moved.
    private let pathMonitor = NWPathMonitor()
    private var pathMonitorStarted = false
    private func startPathMonitor() {
        guard !pathMonitorStarted else { return }
        pathMonitorStarted = true
        pathMonitor.pathUpdateHandler = { [weak self] path in
            let ifaces = path.availableInterfaces.map { "\($0.name)(\($0.type))" }.joined(separator: ",")
            MkpkLog.net("network path: \(String(describing: path.status)) interfaces=[\(ifaces)]")
            guard path.status == .satisfied else { return }
            Task { @MainActor in self?.refreshPublicIP() }
        }
        pathMonitor.start(queue: DispatchQueue.global(qos: .utility))
    }

    @discardableResult
    func importBlob(_ blob: String) async -> Bool {
        do {
            _ = try await store?.importBlob(blob)
            lastError = nil
            await rebuild()
            return true
        } catch {
            lastError = L("Couldn't import the invite: invalid format.", "Не удалось импортировать инвайт: неверный формат.")
            return false
        }
    }

    func importFile(_ url: URL) async {
        do {
            let text = try String(contentsOf: url, encoding: .utf8)
            _ = await importBlob(text)
        } catch {
            lastError = L("Couldn't read the file.", "Не удалось прочитать файл.")
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
            lastError = L("No text in the clipboard.", "В буфере обмена нет текста.")
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

        // Prune keep-open ids whose service is gone, and (re)start the timer so
        // persisted keep-open services get re-opened after a launch.
        let present = Set(groups.flatMap { $0.services.map(\.id) })
        keepOpenIDs.formIntersection(present)
        ensureCountdownTimer()
        refreshMenuState()
        onContentChanged?()
    }

    // MARK: actions

    func knock(_ vm: ServiceVM) {
        MkpkLog.net("knock \(vm.routerAddress) service=\(vm.name) (publicIP=\(self.publicIP ?? "—"))")
        refreshPublicIP()
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

    private func runCheck(_ vm: ServiceVM, note: String, notifyOpen: Bool = true) {
        guard vm.canCheck else { return }
        update(vm.id, .checking)
        Task {
            let res = await Check.run(CheckOptions(host: vm.routerAddress, port: vm.checkPort, timeout: 1, attempts: 6, interval: 0.5))
            MkpkLog.net("check \(vm.routerAddress):\(vm.checkPort) [\(note)] -> \(String(describing: res.status)) (publicIP=\(self.publicIP ?? "—"), openedFor=\(vm.openedForIP ?? "—"))")
            switch res.status {
            case .open where res.viaTunnel:
                // The handshake completed through a VPN/proxy tunnel — the local
                // proxy answers it, so it proves nothing about the real service.
                // Report "unverified" instead of a false "open", but still track the
                // allowed-timeout schedule so keep-open can re-knock before expiry.
                let until = vm.service.allowedTimeout.flatMap(GoDuration.seconds).map { Date().addingTimeInterval($0) }
                update(vm.id, .unverified, openUntil: until)
                appendLog(vm.id, .unverified, note)
                if notifyOpen {
                    notify(L("Can't verify (VPN/proxy)", "Проверка недоступна (VPN/прокси)"),
                           "\(vm.name) · \(vm.routerAddress)")
                }
                ensureCountdownTimer()
            case .open:
                // Arm the countdown from the service's allowed timeout, if known.
                let until = vm.service.allowedTimeout.flatMap(GoDuration.seconds).map { Date().addingTimeInterval($0) }
                update(vm.id, .open, openUntil: until)
                setOpenedIP(vm.id, nil)   // backfilled with the true egress IP by the resolve below
                refreshPublicIP()
                appendLog(vm.id, .open, note)
                if notifyOpen {
                    lastError = nil   // a stale failure banner would contradict the green row
                    notify(L("Access open", "Доступ открыт"), "\(vm.name) · \(vm.routerAddress)")
                }
                ensureCountdownTimer()
            case .closed:
                update(vm.id, .closed, openUntil: nil)
                appendLog(vm.id, .closed, "\(note) · \(res.attempts)×")
                // Fail loudly: "closed" after a manual knock is a failure the user
                // is waiting to hear about, not a neutral state (parity with the
                // Windows client's toast).
                if notifyOpen {
                    lastError = L("\(vm.name): still closed after \(res.attempts) attempts (\(vm.routerAddress):\(vm.checkPort)).",
                                  "\(vm.name): закрыто после \(res.attempts) попыток (\(vm.routerAddress):\(vm.checkPort)).")
                }
            case .error:
                update(vm.id, .closed, openUntil: nil)
                appendLog(vm.id, .timeout, "\(note) · \(res.attempts)×")   // refused/timeout — refined by diagnostics later
                if notifyOpen {
                    let detail = res.lastError.map { " — \($0)" } ?? ""
                    lastError = L("\(vm.name): check failed after \(res.attempts) attempts (\(vm.routerAddress):\(vm.checkPort))\(detail).",
                                  "\(vm.name): проверка не удалась после \(res.attempts) попыток (\(vm.routerAddress):\(vm.checkPort))\(detail).")
                }
            }
        }
    }

    private func appendLog(_ id: String, _ result: LogResult, _ note: String) {
        var arr = logs[id] ?? []
        arr.insert(LogEntry(time: Date(), result: result, note: note), at: 0)
        if arr.count > 20 { arr.removeLast(arr.count - 20) }
        logs[id] = arr
    }

    // MARK: keep-open (#1)

    /// Keep-open needs the allowed_timeout so it knows when to re-knock. Invites
    /// issued before that export lack it → the toggle is unavailable.
    func canKeepOpen(_ vm: ServiceVM) -> Bool { vm.service.allowedTimeout != nil }
    func isKeepOpen(_ id: String) -> Bool { keepOpenIDs.contains(id) }

    func setKeepOpen(_ vm: ServiceVM, _ on: Bool) {
        if on { keepOpenIDs.insert(vm.id) } else { keepOpenIDs.remove(vm.id) }
        renewFailures[vm.id] = 0
        UserDefaults.standard.set(Array(keepOpenIDs), forKey: kKeepOpenKey)
        if on, vm.status != .open { renew(vm) }   // open it now; timer renews near expiry
        ensureCountdownTimer()
        onContentChanged?()
    }

    /// Silent re-knock to hold a service open (or re-open it). Verifies via the
    /// TCP check when available; gives up after 3 consecutive failures.
    private func renew(_ vm: ServiceVM) {
        guard keepOpenIDs.contains(vm.id), !renewingIDs.contains(vm.id) else { return }
        renewingIDs.insert(vm.id)
        Task {
            defer { renewingIDs.remove(vm.id) }
            do {
                try await Knock.perform(Knock.options(router: vm.router, service: vm.service, clientID: vm.clientID))
            } catch {
                recordRenewFailure(vm); return
            }
            let until = vm.service.allowedTimeout.flatMap(GoDuration.seconds).map { Date().addingTimeInterval($0) }
            if vm.canCheck {
                let res = await Check.run(CheckOptions(host: vm.routerAddress, port: vm.checkPort, timeout: 1, attempts: 6, interval: 0.5))
                if res.status == .open, res.viaTunnel {
                    // Re-knocked, but behind a VPN/proxy we can't confirm — don't
                    // claim "open". Keep the schedule so we re-knock before expiry;
                    // not a failure, just unverified.
                    update(vm.id, .unverified, openUntil: until)
                    appendLog(vm.id, .unverified, "keep-open")
                    renewFailures[vm.id] = 0
                    ensureCountdownTimer()
                } else if res.status == .open {
                    update(vm.id, .open, openUntil: until)
                    setOpenedIP(vm.id, nil); refreshPublicIP()   // re-record the egress IP
                    appendLog(vm.id, .open, "keep-open")
                    renewFailures[vm.id] = 0
                    ensureCountdownTimer()
                } else {
                    recordRenewFailure(vm)
                }
            } else {
                update(vm.id, .open, openUntil: until)   // no check → trust the re-knock
                setOpenedIP(vm.id, nil); refreshPublicIP()   // re-record the egress IP
                appendLog(vm.id, .sent, "keep-open")
                renewFailures[vm.id] = 0
                ensureCountdownTimer()
            }
        }
    }

    private func recordRenewFailure(_ vm: ServiceVM) {
        let n = (renewFailures[vm.id] ?? 0) + 1
        renewFailures[vm.id] = n
        appendLog(vm.id, .error, "keep-open")
        if n >= 3 {
            keepOpenIDs.remove(vm.id)
            renewFailures[vm.id] = 0
            UserDefaults.standard.set(Array(keepOpenIDs), forKey: kKeepOpenKey)
            update(vm.id, .error)   // amber attention
            notify(L("Keep-open stopped", "Keep-open остановлен"),
                   L("\(vm.name) · \(vm.routerAddress) — 3 failures in a row", "\(vm.name) · \(vm.routerAddress) — 3 неудачи подряд"))
            onContentChanged?()
        }
    }

    /// Per keep-open service: re-knock when approaching expiry, or re-open when
    /// closed (e.g. after sleep). Skips while a user action or a renew is running.
    private func maintainKeepOpen() {
        for g in groups {
            for svc in g.services where keepOpenIDs.contains(svc.id) && !renewingIDs.contains(svc.id) {
                switch svc.status {
                case .open, .unverified:
                    // Re-knock as expiry approaches. Under a tunnel the state stays
                    // .unverified, but we still keep the access alive on schedule
                    // (renew only fires near expiry, so no per-tick spam).
                    if let until = svc.openUntil {
                        let lead = min(max(TimeInterval(svc.router.bucketSeconds), 15), 30)
                        if now >= until.addingTimeInterval(-lead) { renew(svc) }
                    }
                case .closed, .error, .unknown:
                    renew(svc)   // re-open
                case .knocking, .checking:
                    break        // user action in flight
                }
            }
        }
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
    func quit() { NSApp.terminate(nil) }

    /// Sparkle hook, set by the AppDelegate when the bundle carries an update
    /// feed (release .app only). nil → Settings hides the update button.
    var checkForUpdates: (() -> Void)?

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
        if h > 0 { return m > 0 ? L("\(h) h \(m) min", "\(h) ч \(m) мин") : L("\(h) h", "\(h) ч") }
        if m > 0 { return sec > 0 ? L("\(m) min \(sec) s", "\(m) мин \(sec) с") : L("\(m) min", "\(m) мин") }
        return L("\(sec) s", "\(sec) с")
    }

    private func update(_ id: String, _ status: ServiceStatus, openUntil: Date?? = nil) {
        for gi in groups.indices {
            if let si = groups[gi].services.firstIndex(where: { $0.id == id }) {
                groups[gi].services[si].status = status
                if let openUntil { groups[gi].services[si].openUntil = openUntil }
                refreshMenuState()
                return
            }
        }
    }

    // MARK: countdown

    private var anyOpenWithCountdown: Bool {
        groups.contains { $0.services.contains { $0.status == .open && $0.openUntil != nil } }
    }

    /// The timer runs while any countdown is live OR any keep-open service exists
    /// (so keep-open can re-knock even when a held service has momentarily closed).
    private var timerNeeded: Bool {
        if anyOpenWithCountdown { return true }
        return groups.contains { $0.services.contains { keepOpenIDs.contains($0.id) } }
    }

    private func ensureCountdownTimer() {
        if timerNeeded, countdownTimer == nil {
            let t = Timer(timeInterval: 1, repeats: true) { [weak self] _ in
                Task { @MainActor in self?.tickCountdowns() }
            }
            RunLoop.main.add(t, forMode: .common)
            countdownTimer = t
        } else if !timerNeeded {
            countdownTimer?.invalidate()
            countdownTimer = nil
        }
    }

    private func tickCountdowns() {
        now = Date()
        for gi in groups.indices {
            for si in groups[gi].services.indices where groups[gi].services[si].status == .open {
                if let until = groups[gi].services[si].openUntil, until <= now {
                    let svc = groups[gi].services[si]
                    groups[gi].services[si].status = .closed
                    groups[gi].services[si].openUntil = nil
                    // keep-open will re-open it below — only notify a real close.
                    if !keepOpenIDs.contains(svc.id) {
                        notify(L("Access closed", "Доступ закрыт"), "\(svc.name) · \(svc.routerAddress)")
                    }
                }
            }
        }
        maintainKeepOpen()
        ipTick += 1
        if ipTick >= 60 { ipTick = 0; refreshPublicIP() }   // backstop re-resolve while open
        ensureCountdownTimer()
        refreshMenuState()
    }
    private var ipTick = 0
}
