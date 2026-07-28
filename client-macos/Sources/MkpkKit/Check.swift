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
            if let err = await connectOnce(host: opts.host, port: nwPort, timeout: opts.timeout) {
                lastError = err
                if attempt < attempts {
                    try? await Task.sleep(nanoseconds: UInt64(max(opts.interval, 0) * 1_000_000_000))
                }
            } else {
                return CheckResult(status: .open, host: opts.host, port: opts.port, attempts: attempt, lastError: nil)
            }
        }
        return CheckResult(status: .closed, host: opts.host, port: opts.port, attempts: attempts,
                           lastError: lastError.map { String(describing: $0) })
    }

    /// One TCP connect attempt with a timeout. Returns nil on success, else the error.
    static func connectOnce(host: String, port: NWEndpoint.Port, timeout: TimeInterval) async -> Error? {
        let conn = NWConnection(host: NWEndpoint.Host(host), port: port, using: .tcp)
        let waiter = ConnectWaiter()
        let queue = DispatchQueue(label: "mkpk.check.tcp")
        let err: Error? = await withCheckedContinuation { cont in
            waiter.set(cont)
            conn.stateUpdateHandler = { state in
                switch state {
                case .ready: waiter.finish(nil)
                case .failed(let e): waiter.finish(e)
                case .waiting(let e): waiter.finish(e) // refused / no route → treat as closed
                default: break
                }
            }
            conn.start(queue: queue)
            queue.asyncAfter(deadline: .now() + timeout) { waiter.finish(POSIXError(.ETIMEDOUT)) }
        }
        conn.cancel()
        return err
    }
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
