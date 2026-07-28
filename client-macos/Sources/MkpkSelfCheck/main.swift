import Foundation
import MkpkKit

// A tiny framework-free self-check so the runtime can be verified with just the
// Command Line Tools (`swift run mkpk-selfcheck`). Exits non-zero on any failure.
// The golden token vectors are generated from the Go reference and pin the Swift
// token to the exact RouterOS/Go formula.

// Live mode: `swift run mkpk-selfcheck live <invite-file> <service>` performs a
// real knock + check against the router in the invite. Not part of the
// deterministic self-check (needs the network + a live router).
let args = CommandLine.arguments
if args.count >= 4, args[1] == "live" {
    await runLive(invitePath: args[2], serviceName: args[3])
    exit(0)
}

func runLive(invitePath: String, serviceName: String) async {
    do {
        let inv = try Invite.load(contentsOf: URL(fileURLWithPath: invitePath))
        guard let router = inv.routers.first(where: { $0.services.contains { $0.name == serviceName } }),
              let svc = router.services.first(where: { $0.name == serviceName }) else {
            print("service \(serviceName) not found in invite"); exit(1)
        }
        print("live knock: router=\(router.router) service=\(svc.name) client=\(inv.clientID) check_port=\(svc.checkPort)")
        let opts = Knock.options(router: router, service: svc, clientID: inv.clientID)
        print("knocking…")
        try await Knock.perform(opts)
        print("knock sent; checking \(router.router):\(svc.checkPort) …")
        let res = await Check.run(CheckOptions(host: router.router, port: svc.checkPort, timeout: 1, attempts: 10, interval: 0.5))
        print("result: status=\(res.status.rawValue) attempts=\(res.attempts)\(res.lastError.map { " lastError=\($0)" } ?? "")")
    } catch {
        print("live knock error: \(error)"); exit(1)
    }
}

var failures = 0
@MainActor func check(_ name: String, _ ok: Bool) {
    if ok {
        print("  ok   \(name)")
    } else {
        print("  FAIL \(name)")
        failures += 1
    }
}

// --- token golden vectors (from client/internal/token) ---
let goldens: [(psk: String, service: String, clientID: String, bucket: Int64, expected: String)] = [
    ("test-psk", "ssh", "laptop", 59508396,
     "9f2cb54f14bf75fc17b09bac572de77189765ff98e7131218b3c4e2cff58f32b7fa0cca35f6480b9a07091754833957d3829bbd2485807ffb9286d1d637f4949"),
    ("sPUCpfr4fl-tWQOFTW632l0jrWksEhvJ3ngeuPcRyNU", "web-home", "alice", 59508412,
     "b966ee1d32ef77a995eadf4788a2f86f4fef351300ebd5a50acc6b0fb16d81607a59258c2a0febd6da6f11d68749837b35009dde0defc8b735877b454d5feb76"),
    ("A", "s", "c", 0,
     "6a8a0c599fa8076e555019979378b9abdb2b264d1a78901abe05f2b12b11a57e3729f758eb56d0a03a462c61a37feecefa3268c26ef58c33e48ff794d1f95a51"),
]
print("token golden vectors:")
for g in goldens {
    let got = Token.compute(psk: g.psk, service: g.service, clientID: g.clientID, bucket: g.bucket)
    check("\(g.service)/\(g.clientID)@\(g.bucket)", got == g.expected && got.count == 128)
}

// --- bucket math ---
print("bucket math:")
let base = Date(timeIntervalSince1970: 1_785_252_360) // /30 == 59508412
check("floor", Token.bucket(base, bucketSeconds: 30) == 59508412)
check("same bucket +29s", Token.bucket(base.addingTimeInterval(29), bucketSeconds: 30) == 59508412)
check("next bucket +30s", Token.bucket(base.addingTimeInterval(30), bucketSeconds: 30) == 59508413)

// --- invite decode (synthetic, placeholder host/PSK) ---
print("invite decode:")
let synthBlob = "eyJ2IjoyLCJjbGllbnRfaWQiOiJsYXB0b3AiLCJyb3V0ZXJzIjpbeyJyb3V0ZXIiOiJyb3V0ZXIuZXhhbXBsZS5jb20iLCJidWNrZXRfc2Vjb25kcyI6MzAsInBzayI6InRlc3QtcHNrLTEiLCJzZXJ2aWNlcyI6W3sibmFtZSI6InNzaCIsInN0YWdlMSI6NDEwMTEsInN0YWdlMiI6NDEwMTIsInRva2VuIjo0MTAxMywiY2hlY2tfcG9ydCI6MjJ9LHsibmFtZSI6IndlYiIsInN0YWdlMSI6NDEwMjEsInN0YWdlMiI6NDEwMjIsInRva2VuIjo0MTAyMywiY2hlY2tfcG9ydCI6NDQzfV19XX0"
do {
    let inv = try Invite.decode(blob: "  \n" + synthBlob + "\n")
    check("version", inv.version == 2)
    check("client_id", inv.clientID == "laptop")
    check("router address", inv.routers.first?.router == "router.example.com")
    check("bucket_seconds", inv.routers.first?.bucketSeconds == 30)
    check("services", inv.routers.first?.services.map(\.name) == ["ssh", "web"])
    check("check_port", inv.routers.first?.services.last?.checkPort == 443)
} catch {
    check("decode synthetic invite", false)
    print("    error: \(error)")
}
check("rejects garbage", (try? Invite.decode(blob: "!!!not-base64!!!")) == nil)

// --- check (TCP reachability) ---
print("check (TCP):")
let closed = await Check.run(CheckOptions(host: "127.0.0.1", port: 1, timeout: 0.3, attempts: 1, interval: 0))
check("closed port → closed", closed.status == .closed)
let badPort = await Check.run(CheckOptions(host: "127.0.0.1", port: 0))
check("invalid port → error", badPort.status == .error)
let emptyHost = await Check.run(CheckOptions(host: "", port: 80))
check("empty host → error", emptyHost.status == .error)

// --- invite store (in-memory backend; Keychain needs a signed app) ---
print("invite store:")
// A second synthetic invite for a different client, to test multiple invites.
let synthBlob2 = "eyJ2IjoyLCJjbGllbnRfaWQiOiJwaG9uZSIsInJvdXRlcnMiOlt7InJvdXRlciI6InIyLmV4YW1wbGUuY29tIiwiYnVja2V0X3NlY29uZHMiOjMwLCJwc2siOiJ0ZXN0LXBzay0yIiwic2VydmljZXMiOlt7Im5hbWUiOiJzc2giLCJzdGFnZTEiOjQyMDExLCJzdGFnZTIiOjQyMDEyLCJ0b2tlbiI6NDIwMTMsImNoZWNrX3BvcnQiOjIyfV19XX0"
do {
    let store = try InviteStore(storage: InMemoryInviteStorage())
    _ = try await store.importBlob(synthBlob)
    _ = try await store.importBlob(synthBlob2)
    var n = await store.invites.count
    check("two invites imported", n == 2)
    // Reimport the first (same client+routers) → updates in place, no growth.
    _ = try await store.importBlob("  " + synthBlob + "\n")
    n = await store.invites.count
    check("reimport updates (no dup)", n == 2)
    let routers = await store.routers()
    check("flattened routers", routers.count == 2)
    // Remove one.
    let firstID = await store.invites[0].id
    try await store.remove(id: firstID)
    n = await store.invites.count
    check("remove one", n == 1)
    // Persistence: a new store over the same backend sees what was saved.
    let backend = InMemoryInviteStorage()
    let s1 = try InviteStore(storage: backend)
    _ = try await s1.importBlob(synthBlob)
    let s2 = try InviteStore(storage: backend)
    let reloaded = await s2.invites.count
    check("persists across store instances", reloaded == 1)
} catch {
    check("invite store", false)
    print("    error: \(error)")
}

print("")
if failures == 0 {
    print("all checks passed ✓")
    exit(0)
} else {
    print("\(failures) check(s) FAILED ✗")
    exit(1)
}
