// swift-tools-version: 5.9

import PackageDescription
import Foundation

let agentlySDKPackagePath: String = {
    let override = ProcessInfo.processInfo.environment["AGENTLY_IOS_SDK_PACKAGE_PATH"]?
        .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    return override.isEmpty ? "Packages/AgentlySDKPackage" : override
}()

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
        .package(name: "AgentlySDKPackage", path: agentlySDKPackagePath),
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
            path: "Sources/AgentlyAppFoundation",
            resources: [
                .process("Resources")
            ]
        ),
        .testTarget(
            name: "AgentlyAppFoundationTests",
            dependencies: [
                "AgentlyAppFoundation",
                .product(name: "AgentlySDK", package: "AgentlySDKPackage"),
                .product(name: "ForgeIOSRuntime", package: "ForgeIOSPackage")
            ],
            path: "Tests/AgentlyAppFoundationTests"
        )
    ]
)
