import Foundation
import Network

/// Staged UDP knock — the Swift port of the Go `internal/knock` + the CLI's
/// bucket-age wait and token computation. It sends stage1 then stage2 (payload
/// "x") for `stageDuration`, then the PSK time-token for `tokenDuration`, each
/// repeated at `interval`. Fire-and-forget UDP; the router matches by dst-port
/// and adds the observed source IP to its lists.
public enum Knock {
    public struct Options: Sendable {
        public var router: String
        public var stage1Port: Int
        public var stage2Port: Int
        public var tokenPort: Int
        public var psk: String
        public var service: String
        public var clientID: String
        public var bucketSeconds: Int64
        public var minBucketAge: TimeInterval
        public var interval: TimeInterval
        public var stageDuration: TimeInterval
        public var tokenDuration: TimeInterval
        public var timeout: TimeInterval

        public init(router: String, stage1Port: Int, stage2Port: Int, tokenPort: Int,
                    psk: String, service: String, clientID: String, bucketSeconds: Int64,
                    minBucketAge: TimeInterval = 2, interval: TimeInterval = 0.25,
                    stageDuration: TimeInterval = 2, tokenDuration: TimeInterval = 1,
                    timeout: TimeInterval = 2) {
            self.router = router
            self.stage1Port = stage1Port
            self.stage2Port = stage2Port
            self.tokenPort = tokenPort
            self.psk = psk
            self.service = service
            self.clientID = clientID
            self.bucketSeconds = bucketSeconds
            self.minBucketAge = minBucketAge
            self.interval = interval
            self.stageDuration = stageDuration
            self.tokenDuration = tokenDuration
            self.timeout = timeout
        }
    }

    /// Build knock options from an invite router + one of its services.
    public static func options(router: RouterInvite, service: ServiceInvite, clientID: String) -> Options {
        Options(router: router.router, stage1Port: service.stage1, stage2Port: service.stage2,
                tokenPort: service.token, psk: router.psk, service: service.name,
                clientID: clientID, bucketSeconds: router.bucketSeconds)
    }

    /// Perform the full knock: wait for the bucket to be old enough, compute the
    /// token for the current bucket, then send stage1/stage2/token.
    public static func perform(_ opts: Options) async throws {
        try await waitForBucketAge(bucketSeconds: opts.bucketSeconds, minAge: opts.minBucketAge)
        let bucket = Token.bucket(Date(), bucketSeconds: opts.bucketSeconds)
        let token = Token.compute(psk: opts.psk, service: opts.service, clientID: opts.clientID, bucket: bucket)

        let x = Data("x".utf8)
        try await sendRepeated(host: opts.router, port: opts.stage1Port, payload: x,
                               duration: opts.stageDuration, interval: opts.interval, timeout: opts.timeout)
        try await sendRepeated(host: opts.router, port: opts.stage2Port, payload: x,
                               duration: opts.stageDuration, interval: opts.interval, timeout: opts.timeout)
        try await sendRepeated(host: opts.router, port: opts.tokenPort, payload: Data(token.utf8),
                               duration: opts.tokenDuration, interval: opts.interval, timeout: opts.timeout)
    }

    /// Sleep until the current time bucket is at least `minAge` old, matching the
    /// Go CLI's --min-bucket-age (reduces the risk of sending a token the router
    /// hasn't started accepting yet).
    public static func waitForBucketAge(bucketSeconds: Int64, minAge: TimeInterval) async throws {
        guard minAge >= 0 else { throw KnockError.invalidMinBucketAge }
        let bucketDuration = Double(bucketSeconds)
        guard minAge < bucketDuration else { throw KnockError.minBucketAgeTooLarge }
        let now = Date().timeIntervalSince1970
        let start = (now / bucketDuration).rounded(.down) * bucketDuration
        let age = now - start
        if age < minAge {
            try await Task.sleep(nanoseconds: UInt64((minAge - age) * 1_000_000_000))
        }
    }

    /// Send `payload` to host:port repeatedly for `duration` at `interval`.
    static func sendRepeated(host: String, port: Int, payload: Data,
                             duration: TimeInterval, interval: TimeInterval, timeout: TimeInterval) async throws {
        guard let nwPort = NWEndpoint.Port(rawValue: UInt16(port)) else {
            throw KnockError.invalidPort(port)
        }
        let conn = try await udpConnection(host: host, port: nwPort, timeout: timeout)
        defer { conn.cancel() }
        let deadline = Date().addingTimeInterval(duration)
        while true {
            try await send(conn, payload)
            if Date().addingTimeInterval(interval) >= deadline { break }
            try await Task.sleep(nanoseconds: UInt64(max(interval, 0) * 1_000_000_000))
        }
    }

    static func udpConnection(host: String, port: NWEndpoint.Port, timeout: TimeInterval) async throws -> NWConnection {
        let conn = NWConnection(host: NWEndpoint.Host(host), port: port, using: .udp)
        let waiter = ConnectWaiter()
        let queue = DispatchQueue(label: "mkpk.knock.udp")
        let err: Error? = await withCheckedContinuation { cont in
            waiter.set(cont)
            conn.stateUpdateHandler = { state in
                switch state {
                case .ready: waiter.finish(nil)
                case .failed(let e): waiter.finish(e)
                case .waiting(let e): waiter.finish(e)
                default: break
                }
            }
            conn.start(queue: queue)
            queue.asyncAfter(deadline: .now() + timeout) { waiter.finish(POSIXError(.ETIMEDOUT)) }
        }
        if let err {
            conn.cancel()
            throw err
        }
        return conn
    }

    static func send(_ conn: NWConnection, _ payload: Data) async throws {
        try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, Error>) in
            conn.send(content: payload, completion: .contentProcessed { error in
                if let error {
                    cont.resume(throwing: error)
                } else {
                    cont.resume()
                }
            })
        }
    }
}

public enum KnockError: Error, Equatable {
    case invalidMinBucketAge
    case minBucketAgeTooLarge
    case invalidPort(Int)
}
