// swift-tools-version:6.0
import PackageDescription

let package = Package(
    name: "NodeApp",
    platforms: [.macOS(.v14)],
    dependencies: [
        // The single external library the spec allows (6.1): YAML config.
        .package(url: "https://github.com/jpsim/Yams.git", from: "5.1.0")
    ],
    targets: [
        // C shim: acquire/release atomics for the SPSC ring buffer indices.
        // Kept in-package so NodeApp stays within the spec's dependency budget.
        .target(name: "CAtomics"),
        .target(
            name: "NodeCore",
            dependencies: ["CAtomics", .product(name: "Yams", package: "Yams")],
            swiftSettings: [.swiftLanguageMode(.v5)]
        ),
        .executableTarget(
            name: "NodeApp",
            dependencies: ["NodeCore"],
            swiftSettings: [.swiftLanguageMode(.v5)]
        ),
        .testTarget(
            name: "NodeCoreTests",
            dependencies: ["NodeCore"],
            swiftSettings: [.swiftLanguageMode(.v5)]
        ),
    ]
)
