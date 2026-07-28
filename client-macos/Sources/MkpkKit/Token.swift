import Foundation
import CryptoKit

/// PSK-derived time token — the exact port of the Go `internal/token` formula.
/// The token is `sha512(psk|v1|service|client_id|bucket|psk)` as lowercase hex.
/// It MUST stay byte-identical to the Go reference and RouterOS; the golden test
/// vectors in `TokenTests` pin it.
public enum Token {
    /// Formula version, part of the hashed message (Go: token.Version).
    public static let version = "v1"

    /// The time bucket for a moment: floor(unixSeconds / bucketSeconds).
    /// Matches Go `t.Unix() / bucketSeconds` (integer division, truncation).
    public static func bucket(_ date: Date = Date(), bucketSeconds: Int64) -> Int64 {
        precondition(bucketSeconds > 0, "bucketSeconds must be positive")
        // Unix seconds truncated toward zero, as Go's Unix() yields for real times.
        let unix = Int64(date.timeIntervalSince1970)
        return unix / bucketSeconds
    }

    /// Compute the token for a (psk, service, clientID, bucket) tuple.
    public static func compute(psk: String, service: String, clientID: String, bucket: Int64) -> String {
        let msg = "\(psk)|\(version)|\(service)|\(clientID)|\(bucket)|\(psk)"
        let digest = SHA512.hash(data: Data(msg.utf8))
        return digest.map { String(format: "%02x", $0) }.joined()
    }
}
