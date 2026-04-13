import SwiftUI

public struct SettingsScreen: View {
    @ObservedObject private var runtime: SettingsRuntime
    let onApply: () -> Void

    public init(runtime: SettingsRuntime, onApply: @escaping () -> Void = {}) {
        self.runtime = runtime
        self.onApply = onApply
    }

    public var body: some View {
        Form {
            Section("Connection") {
                TextField("API Base URL", text: $runtime.apiBaseURL)
                    .autocorrectionDisabled()
                if !runtime.normalizedAPIBaseURL.isEmpty {
                    LabeledContent("Normalized", value: runtime.normalizedAPIBaseURL)
                        .font(.footnote)
                }
                Text("Use the workspace root URL. If you paste `/v1` or `/v1/api`, the app removes that automatically.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 8) {
                        ForEach(SettingsRuntime.localPresets, id: \.title) { preset in
                            Button(preset.title) {
                                runtime.applyPreset(preset.value)
                            }
                            .buttonStyle(.bordered)
                        }
                        Button("Clear") {
                            runtime.clearAPIBaseURL()
                        }
                        .buttonStyle(.bordered)
                    }
                }
            }
            Section("Workspace") {
                Text("Choose the active agent from the workspace header after metadata loads. The selection is saved automatically for this workspace.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            Section {
                Button("Apply") {
                    runtime.save()
                    onApply()
                }
                .buttonStyle(.borderedProminent)
            }
        }
        .navigationTitle("Settings")
    }
}
