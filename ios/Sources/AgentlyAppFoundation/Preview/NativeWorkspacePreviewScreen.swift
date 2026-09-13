#if DEBUG
import SwiftUI
import ForgeIOSRuntime
import ForgeIOSUI
#if canImport(UIKit)
import UIKit
#endif

public struct NativeWorkspacePreviewConfiguration: Sendable {
    public let baseURL: URL
    public let windowKey: String
    public let parameters: [String: JSONValue]
    public let sectionID: String?
    public var pageSize: Int? = nil

    public init(baseURL: URL, windowKey: String, parameters: [String: JSONValue], sectionID: String? = nil) {
        self.baseURL = baseURL
        self.windowKey = windowKey
        self.parameters = parameters
        self.sectionID = sectionID
    }

    public static func fromLaunchArguments(_ arguments: [String] = CommandLine.arguments) -> Self? {
        guard arguments.contains("--nativeWorkspacePreview") else { return nil }
        let rawBaseURL = argument("--previewBaseURL", in: arguments) ?? "http://127.0.0.1:8118"
        guard let baseURL = URL(string: rawBaseURL),
              ["127.0.0.1", "localhost"].contains(baseURL.host?.lowercased() ?? "") else {
            return nil
        }
        let windowKey = argument("--previewWindow", in: arguments) ?? "advertiser"
        let rawParameters = argument("--previewParameters", in: arguments) ?? #"{"AdvertiserId":[85141]}"#
        let parameters = (try? JSONDecoder().decode([String: JSONValue].self, from: Data(rawParameters.utf8))) ?? [:]
        var configuration = Self(
            baseURL: baseURL,
            windowKey: windowKey,
            parameters: parameters,
            sectionID: argument("--previewSection", in: arguments)
        )
        if let value = argument("--previewPageSize", in: arguments).flatMap(Int.init), (1...100).contains(value) {
            configuration.pageSize = value
        }
        return configuration
    }

    private static func argument(_ name: String, in arguments: [String]) -> String? {
        arguments.first { $0.hasPrefix(name + "=") }?
            .split(separator: "=", maxSplits: 1)
            .last.map(String.init)
    }
}

/// Debug-only host for exercising synthetic workspace metadata through the
/// same Forge runtime and SwiftUI renderer used by the signed-in app.
public struct NativeWorkspacePreviewScreen: View {
    private let configuration: NativeWorkspacePreviewConfiguration
    @State private var runtime: ForgeRuntime
    @State private var windowState: ForgeRuntime.WindowState?
    @State private var windowContext: WindowContext?
    @State private var errorMessage: String?
    @State private var hasStarted = false
    @State private var presentedWindow: ForgeRuntime.WindowState?

    public init(configuration: NativeWorkspacePreviewConfiguration) {
        self.configuration = configuration
        let target = ForgeTargetContext(
            platform: "ios",
            formFactor: "phone",
            surface: "app",
            capabilities: buildAppleTargetCapabilities()
        )
        _runtime = State(initialValue: ForgeRuntime(targetContext: target, windowMetadataBaseURL: configuration.baseURL))
    }

    public var body: some View {
        NavigationStack {
            Group {
                if let errorMessage {
                    ContentUnavailableView(
                        "Preview unavailable",
                        systemImage: "exclamationmark.triangle",
                        description: Text(errorMessage)
                    )
                } else if let windowState, let metadata = windowState.metadata, let windowContext {
                    WindowContentView(
                        runtime: runtime,
                        window: windowContext,
                        metadata: metadata,
                        contentPadding: 8
                    )
                } else {
                    ProgressView("Loading Advertiser…")
                }
            }
            .safeAreaInset(edge: .top, spacing: 0) {
                Text("Synthetic preview")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 4)
                    .background(.bar)
            }
            .navigationTitle(windowState?.title ?? "Advertiser")
            #if os(iOS)
            .navigationBarTitleDisplayMode(.inline)
            #endif
        }
        .task {
            #if canImport(UIKit)
            await runtime.registerExternalURLHandler { url in
                await MainActor.run { UIApplication.shared.open(url) }
            }
            #endif
            await load()
        }
        .onReceive(NotificationCenter.default.publisher(for: Notification.Name("forgeHostedWorkspaceDidOpen"))) { notification in
            guard let opened = notification.userInfo?["state"] as? ForgeRuntime.WindowState,
                  opened.id != windowState?.id else { return }
            presentedWindow = opened
        }
        .sheet(item: $presentedWindow) { opened in
            NativePreviewDestination(runtime: runtime, window: opened)
        }
    }

    private func load() async {
        guard !hasStarted else { return }
        hasStarted = true
        let targetContext = runtime.targetContext
        await runtime.registerWindowMetadataRequestLoader { request in
            let key = request.windowKey.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? request.windowKey
            var components = URLComponents(
                url: configuration.baseURL.appending(path: "api/windows/\(key)"),
                resolvingAgainstBaseURL: false
            )
            components?.queryItems = [
                URLQueryItem(name: "platform", value: targetContext.platform),
                URLQueryItem(name: "formFactor", value: targetContext.formFactor),
                URLQueryItem(name: "surface", value: targetContext.surface)
            ]
            guard let url = components?.url else { throw URLError(.badURL) }
            let (data, response) = try await URLSession.shared.data(from: url)
            guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
                throw URLError(.badServerResponse)
            }
            let payload = try JSONDecoder().decode(JSONValue.self, from: data)
            var selectedPayload = configuration.sectionID.map { previewSelectingSection($0, in: payload) } ?? payload
            if let size = configuration.pageSize {
                selectedPayload = previewClientPageSize(size, in: selectedPayload)
            }
            let selectedData = try JSONEncoder().encode(selectedPayload)
            let envelope = try JSONDecoder().decode(WindowMetadataEnvelope.self, from: selectedData)
            return MetadataResolver.resolve(envelope.data, for: targetContext)
        }
        let opened = await runtime.openWindow(
            key: configuration.windowKey,
            title: configuration.windowKey.capitalized,
            id: "native-preview:\(configuration.windowKey)",
            parameters: configuration.parameters,
            presentation: "hosted"
        )
        if opened.metadata == nil {
            errorMessage = "Could not load \(configuration.windowKey) from \(configuration.baseURL.absoluteString). Check that the preview service is running and the window is available."
        } else {
            windowState = opened
            windowContext = await runtime.windowContext(id: opened.id)
        }
    }
}


private struct WindowMetadataEnvelope: Decodable {
    let data: WindowMetadata
}

private struct NativePreviewDestination: View {
    let runtime: ForgeRuntime
    let window: ForgeRuntime.WindowState
    @Environment(\.dismiss) private var dismiss
    @State private var context: WindowContext?

    var body: some View {
        NavigationStack {
            Group {
                if let metadata = window.metadata, let context {
                    WindowContentView(runtime: runtime, window: context, metadata: metadata)
                } else if window.metadata == nil {
                    ContentUnavailableView("Window unavailable", systemImage: "exclamationmark.triangle")
                } else {
                    ProgressView()
                }
            }
            .navigationTitle(window.metadata == nil ? window.key.capitalized : window.title)
            .toolbar { Button("Done") { dismiss() } }
            .task { context = await runtime.windowContext(id: window.id) }
        }
    }
}

func previewSelectingSection(_ sectionID: String, in value: JSONValue) -> JSONValue {
    switch value {
    case .array(let values):
        return .array(values.map { previewSelectingSection(sectionID, in: $0) })
    case .object(let object):
        var result = object.mapValues { previewSelectingSection(sectionID, in: $0) }
        if var tabs = result["tabs"]?.objectValue,
           let selected = result["containers"]?.arrayValue?.first(where: { previewContainsSection(sectionID, in: $0) }),
           let selectedID = selected.objectValue?["id"]?.stringValue {
            tabs["defaultSelectedTabId"] = .string(selectedID)
            tabs["selectedTabId"] = .string(selectedID)
            result["tabs"] = .object(tabs)
        }
        return .object(result)
    default:
        return value
    }
}

private func previewContainsSection(_ sectionID: String, in value: JSONValue) -> Bool {
    guard let object = value.objectValue else { return false }
    return object["id"]?.stringValue == sectionID
        || (object["containers"]?.arrayValue ?? []).contains { previewContainsSection(sectionID, in: $0) }
}

func previewClientPageSize(_ size: Int, in value: JSONValue) -> JSONValue {
    switch value {
    case .array(let values): return .array(values.map { previewClientPageSize(size, in: $0) })
    case .object(let values):
        var result = values.mapValues { previewClientPageSize(size, in: $0) }
        if result["paginationMode"]?.stringValue == "client" {
            var paging = result["paging"]?.objectValue ?? [:]
            paging["size"] = .number(Double(size))
            result["paging"] = .object(paging)
        }
        return .object(result)
    default: return value
    }
}

#endif
