import Foundation
import AppKit
import UniformTypeIdentifiers
import MkpkKit

/// The app's observable state: the imported invites projected into router groups
/// with per-service status, plus the knock/check actions. @MainActor so SwiftUI
/// can bind directly.
@MainActor
final class AppModel: ObservableObject {
    enum ServiceStatus: Equatable {
        case unknown, checking, knocking, open, closed, error
    }

    struct ServiceVM: Identifiable {
        let id: String
        let name: String
        let routerAddress: String
        let checkPort: Int      // 0 → check unavailable ("check off")
        var status: ServiceStatus = .unknown
        // Runtime inputs for actions:
        let router: RouterInvite
        let service: ServiceInvite
        let clientID: String
        var canCheck: Bool { checkPort > 0 }
        var addressLabel: String { checkPort > 0 ? "\(routerAddress):\(checkPort)" : "\(routerAddress) · check off" }
    }

    struct RouterGroup: Identifiable {
        let id: String          // router address
        let address: String
        var services: [ServiceVM]
        var reachable: Bool? = nil     // derived from any open/checked service (later: health)
        var clockWarn: Bool = false    // reserved for a future clock hint from the router
    }

    @Published var clientID: String = ""
    @Published var groups: [RouterGroup] = []
    @Published var lastError: String?

    private var store: InviteStore?

    var isEmpty: Bool { groups.isEmpty }

    func load() async {
        // Persistent file backend (works unsigned; the signed build can add the
        // Keychain/iCloud backend). Falls back to in-memory if it can't be created.
        let storage: any InviteStorage = (try? FileInviteStorage()) ?? InMemoryInviteStorage()
        store = try? InviteStore(storage: storage)
        await rebuild()
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

    func remove(routerAddress: String) async {
        // Remove any stored invite that contains only this router; for shared
        // multi-router invites we remove the whole invite (simple, matches the
        // "trash on the router group" affordance for now).
        guard let store else { return }
        for stored in await store.invites {
            if let inv = try? stored.decoded(), inv.routers.contains(where: { $0.router == routerAddress }) {
                try? await store.remove(id: stored.id)
            }
        }
        await rebuild()
    }

    private func rebuild() async {
        guard let store else { return }
        let pairs = await store.routers()
        clientID = (try? pairs.first?.invite.decoded().clientID) ?? ""
        var byRouter: [String: RouterGroup] = [:]
        var order: [String] = []
        for (stored, router) in pairs {
            let cid = (try? stored.decoded().clientID) ?? clientID
            if byRouter[router.router] == nil {
                byRouter[router.router] = RouterGroup(id: router.router, address: router.router, services: [])
                order.append(router.router)
            }
            for svc in router.services {
                let vm = ServiceVM(id: "\(router.router)/\(svc.name)", name: svc.name,
                                   routerAddress: router.router, checkPort: svc.checkPort,
                                   router: router, service: svc, clientID: cid)
                byRouter[router.router]?.services.append(vm)
            }
        }
        groups = order.compactMap { byRouter[$0] }
    }

    // MARK: actions

    func knock(_ vm: ServiceVM) {
        update(vm.id, .knocking)
        Task {
            do {
                try await Knock.perform(Knock.options(router: vm.router, service: vm.service, clientID: vm.clientID))
                if vm.canCheck { check(vm) } else { update(vm.id, .unknown) }
            } catch {
                update(vm.id, .error)
            }
        }
    }

    func check(_ vm: ServiceVM) {
        guard vm.canCheck else { return }
        update(vm.id, .checking)
        Task {
            let res = await Check.run(CheckOptions(host: vm.routerAddress, port: vm.checkPort, timeout: 1, attempts: 6, interval: 0.5))
            update(vm.id, res.status == .open ? .open : .closed)
        }
    }

    private func update(_ id: String, _ status: ServiceStatus) {
        for gi in groups.indices {
            if let si = groups[gi].services.firstIndex(where: { $0.id == id }) {
                groups[gi].services[si].status = status
                return
            }
        }
    }
}
