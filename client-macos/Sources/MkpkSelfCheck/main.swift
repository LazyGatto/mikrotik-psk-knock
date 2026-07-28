import Foundation
import MkpkKit

// A tiny framework-free self-check so the runtime can be verified with just the
// Command Line Tools (`swift run mkpk-selfcheck`). Exits non-zero on any failure.
// The golden token vectors are generated from the Go reference and pin the Swift
// token to the exact RouterOS/Go formula.

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

print("")
if failures == 0 {
    print("all checks passed ✓")
    exit(0)
} else {
    print("\(failures) check(s) FAILED ✗")
    exit(1)
}
