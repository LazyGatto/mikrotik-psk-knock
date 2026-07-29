import Foundation
import Network

/// Best-effort public-IP resolution, mirroring powerlevel10k's tiered approach:
/// 1) DNS to OpenDNS (`myip.opendns.com @resolver1.opendns.com`) — fast, no HTTP,
///    the resolver echoes the querying client's public IPv4;
/// 2) HTTP fallback to `v4.ident.me` (IPv4-only host);
/// 3) HTTP fallback to `ipv4.icanhazip.com` (IPv4-only host).
/// Deliberately NOT using ipify — it has returned the wrong egress address here.
/// The DNS path reuses our Network stack (same as the knock) and needs only the
/// outbound-UDP the app already requires.
public enum PublicIP {
    public static func resolve() async -> String? {
        if let ip = await dnsOpenDNS() { return ip }
        for url in ["https://v4.ident.me/", "https://ipv4.icanhazip.com/"] {
            if let ip = await http(url) { return ip }
        }
        return nil
    }

    // MARK: HTTP

    private static func http(_ s: String) async -> String? {
        guard let url = URL(string: s) else { return nil }
        var req = URLRequest(url: url)
        req.timeoutInterval = 5
        req.cachePolicy = .reloadIgnoringLocalCacheData
        guard let (data, _) = try? await URLSession.shared.data(for: req) else { return nil }
        let ip = String(decoding: data, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
        return isValidIP(ip) ? ip : nil
    }

    // MARK: DNS (myip.opendns.com → our public IPv4)

    private static func dnsOpenDNS() async -> String? {
        let query = buildQuery(name: "myip.opendns.com")
        // resolver1.opendns.com
        guard let resp = await udpExchange(host: "208.67.222.222", port: 53, payload: query, timeout: 3) else { return nil }
        return parseA(resp)
    }

    private static func buildQuery(name: String) -> Data {
        var d = Data()
        let id = UInt16.random(in: 0...UInt16.max)
        d.append(UInt8(id >> 8)); d.append(UInt8(id & 0xFF))
        d.append(contentsOf: [0x01, 0x00])                 // flags: recursion desired
        d.append(contentsOf: [0x00, 0x01])                 // QDCOUNT = 1
        d.append(contentsOf: [0x00, 0x00, 0x00, 0x00, 0x00, 0x00]) // AN/NS/AR = 0
        for label in name.split(separator: ".") {
            let bytes = Array(label.utf8)
            d.append(UInt8(bytes.count)); d.append(contentsOf: bytes)
        }
        d.append(0x00)                                     // root label
        d.append(contentsOf: [0x00, 0x01])                 // QTYPE = A
        d.append(contentsOf: [0x00, 0x01])                 // QCLASS = IN
        return d
    }

    private static func parseA(_ data: Data) -> String? {
        let b = [UInt8](data)
        guard b.count > 12 else { return nil }
        let ancount = Int(b[6]) << 8 | Int(b[7])
        guard ancount > 0 else { return nil }
        var i = 12
        // Skip the question's QNAME.
        while i < b.count, b[i] != 0 {
            if b[i] & 0xC0 == 0xC0 { i += 1; break }
            i += Int(b[i]) + 1
        }
        i += 1        // terminating null
        i += 4        // QTYPE + QCLASS
        for _ in 0..<ancount {
            // NAME — a pointer (2 bytes) or a sequence of labels.
            if i < b.count, b[i] & 0xC0 == 0xC0 { i += 2 } else {
                while i < b.count, b[i] != 0 { i += Int(b[i]) + 1 }
                i += 1
            }
            guard i + 10 <= b.count else { return nil }
            let type = Int(b[i]) << 8 | Int(b[i + 1]); i += 2
            i += 2   // CLASS
            i += 4   // TTL
            let rdlen = Int(b[i]) << 8 | Int(b[i + 1]); i += 2
            guard i + rdlen <= b.count else { return nil }
            if type == 1, rdlen == 4 {
                return "\(b[i]).\(b[i + 1]).\(b[i + 2]).\(b[i + 3])"
            }
            i += rdlen
        }
        return nil
    }

    private static func isValidIP(_ s: String) -> Bool {
        let parts = s.split(separator: ".")
        if parts.count == 4 {
            return parts.allSatisfy { Int($0).map { $0 >= 0 && $0 <= 255 } ?? false }
        }
        return s.contains(":")   // allow an IPv6 answer from the HTTP fallback
    }

    // MARK: UDP one-shot exchange

    private static func udpExchange(host: String, port: UInt16, payload: Data, timeout: TimeInterval) async -> Data? {
        await withCheckedContinuation { (cont: CheckedContinuation<Data?, Never>) in
            guard let nwPort = NWEndpoint.Port(rawValue: port) else { cont.resume(returning: nil); return }
            let conn = NWConnection(host: NWEndpoint.Host(host), port: nwPort, using: .udp)
            let box = UDPResumeBox(cont)
            // The box guarantees a single resume, so the timeout can fire
            // unconditionally — if the response already arrived it's a no-op.
            DispatchQueue.global().asyncAfter(deadline: .now() + timeout) {
                box.finish(nil); conn.cancel()
            }
            conn.stateUpdateHandler = { state in
                switch state {
                case .ready:
                    conn.send(content: payload, completion: .contentProcessed { _ in
                        conn.receiveMessage { data, _, _, _ in
                            box.finish(data); conn.cancel()
                        }
                    })
                case .failed, .cancelled:
                    box.finish(nil)
                default:
                    break
                }
            }
            conn.start(queue: DispatchQueue.global())
        }
    }
}

/// Single-resume guard for the UDP continuation (timeout vs response race).
private final class UDPResumeBox: @unchecked Sendable {
    private var cont: CheckedContinuation<Data?, Never>?
    private let lock = NSLock()
    init(_ c: CheckedContinuation<Data?, Never>) { cont = c }
    func finish(_ value: Data?) {
        lock.lock(); defer { lock.unlock() }
        cont?.resume(returning: value); cont = nil
    }
}
