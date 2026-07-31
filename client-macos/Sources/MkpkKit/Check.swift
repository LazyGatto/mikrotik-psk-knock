import Foundation
import Network

/// TCP reachability check of a service endpoint — the Swift port of the Go
/// `internal/servicecheck`. Tries to connect up to `attempts` times with
/// `interval` between; the first success is `open`, all failures are `closed`.
public enum CheckStatus: String, Sendable {
    case open, closed, error
}

public struct CheckOptions: Sendable {
    public var host: String
    public var port: Int
    public var timeout: TimeInterval
    public var attempts: Int
    public var interval: TimeInterval

    public init(host: String, port: Int, timeout: TimeInterval = 1, attempts: Int = 10, interval: TimeInterval = 0.5) {
        self.host = host
        self.port = port
        self.timeout = timeout
        self.attempts = attempts
        self.interval = interval
    }
}

public struct CheckResult: Sendable {
    public let status: CheckStatus
    public let host: String
    public let port: Int
    public let attempts: Int
    public let lastError: String?
    /// The successful connection went through a VPN/transparent-proxy tunnel, so a
    /// TCP handshake proves nothing about the real service (the local proxy answers
    /// it). `open` + `viaTunnel` must be treated as UNVERIFIED, not open.
    public let viaTunnel: Bool
    /// Local endpoint of the successful connection (e.g. "198.18.0.1:54870") — for
    /// diagnostics.
    public let localEndpoint: String?

    init(status: CheckStatus, host: String, port: Int, attempts: Int, lastError: String?,
         viaTunnel: Bool = false, localEndpoint: String? = nil) {
        self.status = status
        self.host = host
        self.port = port
        self.attempts = attempts
        self.lastError = lastError
        self.viaTunnel = viaTunnel
        self.localEndpoint = localEndpoint
    }
}

public enum Check {
    public static func run(_ opts: CheckOptions) async -> CheckResult {
        guard !opts.host.isEmpty else {
            return CheckResult(status: .error, host: opts.host, port: opts.port, attempts: 0, lastError: "host is required")
        }
        guard opts.port > 0, opts.port <= 65535, let nwPort = NWEndpoint.Port(rawValue: UInt16(opts.port)) else {
            return CheckResult(status: .error, host: opts.host, port: opts.port, attempts: 0, lastError: "port must be between 1 and 65535")
        }
        let attempts = max(opts.attempts, 1)
        var lastError: Error?
        for attempt in 1...attempts {
            let probe = TunnelProbe()
            if let err = await connectOnce(host: opts.host, port: nwPort, timeout: opts.timeout, probe: probe) {
                lastError = err
                if attempt < attempts {
                    try? await Task.sleep(nanoseconds: UInt64(max(opts.interval, 0) * 1_000_000_000))
                }
            } else {
                let (via, local) = probe.get()
                return CheckResult(status: .open, host: opts.host, port: opts.port, attempts: attempt,
                                   lastError: nil, viaTunnel: via, localEndpoint: local)
            }
        }
        return CheckResult(status: .closed, host: opts.host, port: opts.port, attempts: attempts,
                           lastError: lastError.map { String(describing: $0) })
    }

    /// One TCP connect attempt with a timeout. Returns nil on success, else the
    /// error. On success, records into `probe` whether the connection used a
    /// VPN/proxy tunnel (so the caller can flag the result as unverified).
    static func connectOnce(host: String, port: NWEndpoint.Port, timeout: TimeInterval, probe: TunnelProbe) async -> Error? {
        let conn = NWConnection(host: NWEndpoint.Host(host), port: port, using: .tcp)
        let waiter = ConnectWaiter()
        let queue = DispatchQueue(label: "mkpk.check.tcp")
        let err: Error? = await withCheckedContinuation { cont in
            waiter.set(cont)
            conn.stateUpdateHandler = { state in
                switch state {
                case .ready:
                    let p = conn.currentPath
                    let local = p?.localEndpoint.map { "\($0)" }
                    // Tunnel if the path uses a utun-type interface (.other) or the
                    // local endpoint is a fake-IP / CGNAT address a proxy hands out.
                    let via = (p?.usesInterfaceType(.other) ?? false) || isTunnelEndpoint(local)
                    probe.set(via: via, local: local)
                    if MkpkLog.verbose {
                        let ifs = p?.availableInterfaces.map { "\($0.name)(\($0.type))" }.joined(separator: ",") ?? "?"
                        MkpkLog.net("check→\(host):\(port.rawValue) READY via [\(ifs)] local=\(local ?? "?") tunnel=\(via)")
                    }
                    waiter.finish(nil)
                case .failed(let e):
                    MkpkLog.net("check→\(host):\(port.rawValue) failed: \(String(describing: e))")
                    waiter.finish(e)
                case .waiting(let e): // refused / no route → treat as closed
                    MkpkLog.net("check→\(host):\(port.rawValue) waiting: \(String(describing: e))")
                    waiter.finish(e)
                default: break
                }
            }
            conn.start(queue: queue)
            queue.asyncAfter(deadline: .now() + timeout) { waiter.finish(POSIXError(.ETIMEDOUT)) }
        }
        conn.cancel()
        return err
    }

    /// Whether a local endpoint description ("198.18.0.1:54870") is a fake-IP /
    /// CGNAT address typically handed out by a VPN/transparent proxy TUN:
    /// 198.18.0.0/15 (benchmark range used by Clash/Surge/sing-box) or
    /// 100.64.0.0/10 (CGNAT, e.g. Tailscale).
    static func isTunnelEndpoint(_ desc: String?) -> Bool {
        guard let desc, !desc.contains("["),                       // skip IPv6
              let host = desc.split(separator: ":").first else { return false }
        if host.hasPrefix("198.18.") || host.hasPrefix("198.19.") { return true }
        let parts = host.split(separator: ".")
        if parts.count == 4, parts[0] == "100", let o2 = Int(parts[1]), (64...127).contains(o2) { return true }
        return false
    }
}

/// Carries tunnel-detection info out of the connect state handler (which runs on
/// the connection's queue) to the awaiting caller.
final class TunnelProbe: @unchecked Sendable {
    private let lock = NSLock()
    private var _via = false
    private var _local: String?
    func set(via: Bool, local: String?) { lock.lock(); _via = via; _local = local; lock.unlock() }
    func get() -> (Bool, String?) { lock.lock(); defer { lock.unlock() }; return (_via, _local) }
}

/// Resumes a connect continuation exactly once, from whichever fires first
/// (state handler or timeout). Shared by Check and Knock.
final class ConnectWaiter: @unchecked Sendable {
    private let lock = NSLock()
    private var done = false
    private var cont: CheckedContinuation<Error?, Never>?

    func set(_ c: CheckedContinuation<Error?, Never>) {
        lock.lock(); defer { lock.unlock() }
        cont = c
    }

    func finish(_ err: Error?) {
        lock.lock(); defer { lock.unlock() }
        if done { return }
        done = true
        cont?.resume(returning: err)
        cont = nil
    }
}
