import Foundation

/// Plain-file invite storage: a JSON array under Application Support (0600). Used
/// for local persistence and as the backend that works in an unsigned dev build
/// (the Keychain backend needs a signed app). iCloud sync uses the Keychain one.
public final class FileInviteStorage: InviteStorage, @unchecked Sendable {
    private let url: URL
    private let lock = NSLock()

    public init(url: URL) {
        self.url = url
    }

    /// Default location: ~/Library/Application Support/<appName>/invites.json.
    public convenience init(appName: String = "mkpk") throws {
        let base = try FileManager.default.url(for: .applicationSupportDirectory, in: .userDomainMask,
                                               appropriateFor: nil, create: true)
        let dir = base.appendingPathComponent(appName, isDirectory: true)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true,
                                                attributes: [.posixPermissions: 0o700])
        self.init(url: dir.appendingPathComponent("invites.json"))
    }

    public func load() throws -> [StoredInvite] {
        lock.lock(); defer { lock.unlock() }
        guard FileManager.default.fileExists(atPath: url.path) else { return [] }
        let data = try Data(contentsOf: url)
        return try JSONDecoder().decode([StoredInvite].self, from: data)
    }

    public func save(_ invites: [StoredInvite]) throws {
        lock.lock(); defer { lock.unlock() }
        let data = try JSONEncoder().encode(invites)
        try data.write(to: url, options: .atomic)
        try? FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: url.path)
    }
}
