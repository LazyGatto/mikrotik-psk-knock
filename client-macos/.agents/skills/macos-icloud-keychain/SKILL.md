# SKILL: iCloud Keychain sync (secrets across devices)

We sync imported invites (which contain PSK secrets) across the user's Macs. Do
it through the **Keychain with `kSecAttrSynchronizable = true`**, which uses
iCloud Keychain — secrets stay encrypted end-to-end. Never put secrets in
`NSUbiquitousKeyValueStore` / plist / plain iCloud.

## Entitlements

The signed app needs:
- `keychain-access-groups` — a group id (e.g. `$(TeamIdentifierPrefix)ru.eg23.mkpk`).
- For iCloud Keychain specifically, no separate iCloud container is required —
  synchronizable generic-password items sync automatically once the app is signed
  with a keychain-access-group and the user has iCloud Keychain enabled.

Example `release.entitlements` fragment:

```xml
<key>keychain-access-groups</key>
<array>
  <string>$(AppIdentifierPrefix)ru.eg23.mkpk.client</string>
</array>
```

## Storing a syncable item

- `kSecClass = kSecClassGenericPassword`, a stable `kSecAttrService`/`kSecAttrAccount`.
- `kSecAttrSynchronizable = kCFBooleanTrue` on **every** query (add/update/copy/
  delete) — a synchronizable item and a non-synchronizable one with the same
  service/account are distinct.
- `kSecAttrAccessible = kSecAttrAccessibleAfterFirstUnlock` (a `...ThisDeviceOnly`
  accessibility does **not** sync).

`KeychainInviteStorage` in MkpkKit implements exactly this; a `synchronizable`
flag toggles the iCloud behaviour, so a user can opt out.

## Testing

Keychain access needs a signed app with the entitlement — an unsigned `swift run`
binary can't use it (it errors / prompts). Test store logic with
`InMemoryInviteStorage`; verify the real Keychain path in the signed app.
