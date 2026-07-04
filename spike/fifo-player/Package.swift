// swift-tools-version:5.10
import PackageDescription

let package = Package(
    name: "fifo-player-spike",
    platforms: [.macOS(.v14)],
    targets: [
        .executableTarget(name: "fifo-player-spike", path: "Sources")
    ]
)
