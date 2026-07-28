import Foundation
import Security

/// Keychain-backed invite storage: the invite list (blobs contain the PSK) is
/// stored as one generic-password item. With `synchronizable = true` the item
/// rides **iCloud Keychain**, so invites sync across the user's Macs — secrets
/// stay encrypted at rest and in transit, never plain iCloud KV.
///
/// Note: Keychain access requires a signed app with a keychain-access group; an
/// unsigned `swift run` binary can't use it. Tests use InMemoryInviteStorage.
public final class KeychainInviteStorage: InviteStorage, @unchecked Sendable {
    private let service: String
    private let account = "invites"
    private let synchronizable: Bool

    public init(service: String = "ru.eg23.mkpk.client", synchronizable: Bool = false) {
        self.service = service
        self.synchronizable = synchronizable
    }

    private func baseQuery() -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecAttrSynchronizable as String: synchronizable ? kCFBooleanTrue! : kCFBooleanFalse!,
        ]
    }

    public func load() throws -> [StoredInvite] {
        var query = baseQuery()
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)
        if status == errSecItemNotFound { return [] }
        guard status == errSecSuccess, let data = item as? Data else {
            throw KeychainError.unexpectedStatus(status)
        }
        return try JSONDecoder().decode([StoredInvite].self, from: data)
    }

    public func save(_ invites: [StoredInvite]) throws {
        let data = try JSONEncoder().encode(invites)
        let query = baseQuery()
        let update: [String: Any] = [kSecValueData as String: data]
        let status = SecItemUpdate(query as CFDictionary, update as CFDictionary)
        switch status {
        case errSecSuccess:
            return
        case errSecItemNotFound:
            var add = query
            add[kSecValueData as String] = data
            // AfterFirstUnlock (not ...ThisDeviceOnly) so the item can sync.
            add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
            let addStatus = SecItemAdd(add as CFDictionary, nil)
            guard addStatus == errSecSuccess else {
                throw KeychainError.unexpectedStatus(addStatus)
            }
        default:
            throw KeychainError.unexpectedStatus(status)
        }
    }
}

public enum KeychainError: Error, Equatable {
    case unexpectedStatus(OSStatus)
}
