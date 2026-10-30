// swift-tools-version:5.9
import PackageDescription

// What the Apple apps share that doesn't depend on any screen: addresses, the
// server's data and the API. It also builds on Linux, so CI can test it anywhere.
let package = Package(
    name: "WittyKit",
    platforms: [.iOS(.v16), .tvOS(.v16), .macOS(.v13)],
    products: [.library(name: "WittyKit", targets: ["WittyKit"])],
    targets: [
        .target(name: "WittyKit"),
        .testTarget(name: "WittyKitTests", dependencies: ["WittyKit"]),
    ]
)
