// swift-tools-version: 5.9

import PackageDescription

let package = Package(
    name: "AgentlyIOSFoundation",
    platforms: [
        .iOS(.v17),
        .macOS(.v14)
    ],
    products: [
        .library(
            name: "AgentlyAppFoundation",
            targets: ["AgentlyAppFoundation"]
        )
    ],
    dependencies: [
        .package(path: "Packages/AgentlySDKPackage"),
        .package(path: "Packages/ForgeIOSPackage")
    ],
    targets: [
        .target(
            name: "AgentlyAppFoundation",
            dependencies: [
                .product(name: "AgentlySDK", package: "AgentlySDKPackage"),
                .product(name: "ForgeIOSRuntime", package: "ForgeIOSPackage"),
                .product(name: "ForgeIOSUI", package: "ForgeIOSPackage")
            ],
            path: "Sources/AgentlyAppFoundation"
        ),
        .testTarget(
            name: "AgentlyAppFoundationTests",
            dependencies: [
                "AgentlyAppFoundation",
                .product(name: "AgentlySDK", package: "AgentlySDKPackage")
            ],
            path: "Tests/AgentlyAppFoundationTests"
        )
    ]
)
