// swift-tools-version: 6.0
import PackageDescription

// mkpk client (invite-recipient) — native macOS menu-bar app.
// MkpkKit is the runtime core (invite parsing, PSK time-token, staged UDP knock,
// TCP check), reimplemented from the Go reference and pinned to it with golden
// test vectors. The menu-bar app target is added once the UI mockups land.
let package = Package(
    name: "mkpk-client",
    platforms: [.macOS(.v13)],
    products: [
        .library(name: "MkpkKit", targets: ["MkpkKit"]),
        // Self-check runner: exercises the runtime (golden token vectors, invite
        // decode) with plain `swift run mkpk-selfcheck`, so correctness is
        // verifiable with Command Line Tools only. A real `swift test` suite
        // (Swift Testing / XCTest) needs full Xcode and is added once available.
        .executable(name: "mkpk-selfcheck", targets: ["MkpkSelfCheck"]),
        // The menu-bar app (invite recipient). SwiftUI in an AppKit NSStatusItem
        // + NSPanel popover. Run: `swift run MkpkApp` (dev) — a proper .app bundle
        // (LSUIElement, signed) is assembled by script/ for distribution.
        .executable(name: "MkpkApp", targets: ["MkpkApp"]),
    ],
    targets: [
        .target(
            name: "MkpkKit",
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
        .executableTarget(
            name: "MkpkSelfCheck",
            dependencies: ["MkpkKit"],
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
        .executableTarget(
            name: "MkpkApp",
            dependencies: ["MkpkKit"],
            resources: [.process("Resources")],
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
    ]
)
