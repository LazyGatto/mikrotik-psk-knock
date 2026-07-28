import Foundation
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

    private var store: InviteStore?

    /// Dev seed: two synthetic invites (placeholder hosts). Replaced by real
    /// import + a persistent backend as the UI grows.
    private static let seedBlobs = [
        "eyJ2IjoyLCJjbGllbnRfaWQiOiJsYXB0b3AiLCJyb3V0ZXJzIjpbeyJyb3V0ZXIiOiJyb3V0ZXIuZXhhbXBsZS5jb20iLCJidWNrZXRfc2Vjb25kcyI6MzAsInBzayI6InRlc3QtcHNrLTEiLCJzZXJ2aWNlcyI6W3sibmFtZSI6InNzaC1ob21lIiwic3RhZ2UxIjo0MTAxMSwic3RhZ2UyIjo0MTAxMiwidG9rZW4iOjQxMDEzLCJjaGVja19wb3J0IjoyMiwiYWxsb3dlZF90aW1lb3V0IjoiM20ifSx7Im5hbWUiOiJ3ZWItaG9tZSIsInN0YWdlMSI6NDEwMjEsInN0YWdlMiI6NDEwMjIsInRva2VuIjo0MTAyMywiY2hlY2tfcG9ydCI6NDQzfSx7Im5hbWUiOiJ3ZyIsInN0YWdlMSI6NDEwMzEsInN0YWdlMiI6NDEwMzIsInRva2VuIjo0MTAzMywiY2hlY2tfcG9ydCI6MH1dfV19",
    ]

    func load() async {
        let storage = InMemoryInviteStorage()
        let s = try? InviteStore(storage: storage)
        for b in Self.seedBlobs {
            _ = try? await s?.importBlob(b)
        }
        store = s
        await rebuild()
    }

    func importBlob(_ blob: String) async {
        _ = try? await store?.importBlob(blob)
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
