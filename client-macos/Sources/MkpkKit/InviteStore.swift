import Foundation
import CryptoKit

/// One imported invite as persisted: the raw blob plus when it was imported.
/// The decoded `Invite` and stable `id` are derived on demand so persistence
/// stays a plain list of blobs.
public struct StoredInvite: Sendable, Codable, Equatable {
    public let blob: String
    public let importedAt: Date

    public init(blob: String, importedAt: Date) {
        self.blob = blob.trimmingCharacters(in: .whitespacesAndNewlines)
        self.importedAt = importedAt
    }

    public func decoded() throws -> Invite {
        try Invite.decode(blob: blob)
    }

    /// Stable identity used for de-dup / reimport: a client's invite for the same
    /// set of routers replaces the old one (e.g. after a PSK rotation). Falls back
    /// to hashing the blob if it can't be decoded.
    public var id: String {
        if let inv = try? decoded() {
            let material = inv.clientID + "\n" + inv.routers.map(\.router).sorted().joined(separator: "\n")
            return Self.sha256Hex(material)
        }
        return Self.sha256Hex(blob)
    }

    static func sha256Hex(_ s: String) -> String {
        SHA256.hash(data: Data(s.utf8)).map { String(format: "%02x", $0) }.joined()
    }
}

/// Persistence backend for imported invites. The app uses the Keychain backend
/// (secrets at rest, optional iCloud sync); tests use the in-memory one.
public protocol InviteStorage: Sendable {
    func load() throws -> [StoredInvite]
    func save(_ invites: [StoredInvite]) throws
}

public final class InMemoryInviteStorage: InviteStorage, @unchecked Sendable {
    private let lock = NSLock()
    private var data: [StoredInvite]
    public init(_ initial: [StoredInvite] = []) { data = initial }
    public func load() throws -> [StoredInvite] {
        lock.lock(); defer { lock.unlock() }
        return data
    }
    public func save(_ invites: [StoredInvite]) throws {
        lock.lock(); defer { lock.unlock() }
        data = invites
    }
}

/// Manages the collection of imported invites: add (decoding + validating),
/// reimport (updates the matching one), remove. Multiple `.mkpk` are supported —
/// distinct invites live side by side. Backed by an `InviteStorage`.
public actor InviteStore {
    private let storage: any InviteStorage
    public private(set) var invites: [StoredInvite]

    public init(storage: any InviteStorage) throws {
        self.storage = storage
        self.invites = try storage.load()
    }

    /// Import a blob string. Decodes to validate; a blob with the same identity
    /// (same client + routers) replaces the existing entry (reimport). Returns
    /// the stored invite.
    @discardableResult
    public func importBlob(_ blob: String) throws -> StoredInvite {
        _ = try Invite.decode(blob: blob) // validate before storing
        let stored = StoredInvite(blob: blob, importedAt: Date())
        if let idx = invites.firstIndex(where: { $0.id == stored.id }) {
            invites[idx] = stored
        } else {
            invites.append(stored)
        }
        try storage.save(invites)
        return stored
    }

    public func remove(id: String) throws {
        invites.removeAll { $0.id == id }
        try storage.save(invites)
    }

    public func removeAll() throws {
        invites.removeAll()
        try storage.save(invites)
    }

    /// All (invite, router) pairs across every imported invite, for the UI.
    public func routers() -> [(invite: StoredInvite, router: RouterInvite)] {
        invites.flatMap { stored -> [(StoredInvite, RouterInvite)] in
            guard let inv = try? stored.decoded() else { return [] }
            return inv.routers.map { (stored, $0) }
        }
    }
}
