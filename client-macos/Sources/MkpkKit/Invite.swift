import Foundation

/// The per-user invite blob — a base64url-encoded JSON matching the Go
/// `internal/invite` v2 format. It carries only what a client needs to knock:
/// the public router address, the PSK, and each service's ports.
public struct Invite: Codable, Sendable, Equatable {
    public let version: Int
    public let clientID: String
    public let routers: [RouterInvite]

    enum CodingKeys: String, CodingKey {
        case version = "v"
        case clientID = "client_id"
        case routers
    }
}

public struct RouterInvite: Codable, Sendable, Equatable {
    /// Public address end users knock (the only address in the blob).
    public let router: String
    public let bucketSeconds: Int64
    public let psk: String
    public let services: [ServiceInvite]

    enum CodingKeys: String, CodingKey {
        case router
        case bucketSeconds = "bucket_seconds"
        case psk
        case services
    }
}

public struct ServiceInvite: Codable, Sendable, Equatable {
    /// service_name — part of the token formula.
    public let name: String
    public let stage1: Int
    public let stage2: Int
    public let token: Int
    /// External port for the post-knock TCP check (0 → no auto-check possible).
    public let checkPort: Int
    /// Effective TTL of the allowed entry after a knock (e.g. "3m"), so the UI can
    /// show how long the port stays open. Informational; the router is authority.
    /// Absent on invites from older exporters.
    public let allowedTimeout: String?

    enum CodingKeys: String, CodingKey {
        case name, stage1, stage2, token
        case checkPort = "check_port"
        case allowedTimeout = "allowed_timeout"
    }
}

public enum InviteError: Error, Equatable {
    case notBase64URL
    case decode(String)
}

extension Invite {
    /// Decode an invite from its base64url blob string (as produced by
    /// `mkpk-provision export`). Whitespace/newlines are trimmed.
    public static func decode(blob: String) throws -> Invite {
        let trimmed = blob.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let data = base64URLDecode(trimmed) else {
            throw InviteError.notBase64URL
        }
        do {
            return try JSONDecoder().decode(Invite.self, from: data)
        } catch {
            throw InviteError.decode(String(describing: error))
        }
    }

    /// Read an invite from a file (a `.mkpk` produced by export).
    public static func load(contentsOf url: URL) throws -> Invite {
        let text = try String(contentsOf: url, encoding: .utf8)
        return try decode(blob: text)
    }
}

/// Decode base64url (RFC 4648 §5, no padding) into Data, tolerating optional
/// padding. Returns nil on invalid input.
func base64URLDecode(_ s: String) -> Data? {
    var str = s.replacingOccurrences(of: "-", with: "+")
        .replacingOccurrences(of: "_", with: "/")
    let remainder = str.count % 4
    if remainder > 0 {
        str += String(repeating: "=", count: 4 - remainder)
    }
    return Data(base64Encoded: str)
}
