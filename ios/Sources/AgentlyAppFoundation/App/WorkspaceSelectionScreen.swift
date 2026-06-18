import SwiftUI
#if canImport(UIKit)
import UIKit
#endif

struct WorkspaceSelectionScreen: View {
    @ObservedObject private var settingsRuntime: SettingsRuntime
    @State private var selectedEndpoint: String
    @State private var isStarting = false

    private let onContinue: (WorkspaceEndpointOption) -> Void

    init(
        settingsRuntime: SettingsRuntime,
        onContinue: @escaping (WorkspaceEndpointOption) -> Void
    ) {
        self.settingsRuntime = settingsRuntime
        let initial = settingsRuntime.selectedWorkspacePreset
            ?? SettingsRuntime.workspacePresets.first
            ?? WorkspaceEndpointOption(title: "Workspace", subtitle: "", value: "")
        self._selectedEndpoint = State(initialValue: initial.value)
        self.onContinue = onContinue
    }

    var body: some View {
        ZStack {
            workspaceOnboardingBackground()
                .ignoresSafeArea()
            ScrollView {
                VStack(alignment: .leading, spacing: 28) {
                    header
                    workspacePicker
                    continueButton
                }
                .frame(maxWidth: 560, alignment: .leading)
                .padding(.horizontal, 24)
                .padding(.vertical, 34)
            }
        }
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 22) {
            ViantLogoMark()
            VStack(alignment: .leading, spacing: 10) {
                Text("Choose your workspace")
                    .font(.system(.largeTitle, design: .rounded).weight(.bold))
                    .foregroundStyle(.primary)
                Text("Connect Agently to the workspace you use for planning, forecasting, and agent workflows.")
                    .font(.body)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    private var workspacePicker: some View {
        VStack(alignment: .leading, spacing: 16) {
            ForEach(SettingsRuntime.workspacePresets) { preset in
                workspaceEndpointOptionRow(preset)
            }
        }
    }

    private func workspaceEndpointOptionRow(_ option: WorkspaceEndpointOption) -> some View {
        Button {
            selectedEndpoint = option.value
        } label: {
            HStack(alignment: .center, spacing: 12) {
                Image(systemName: selectedEndpoint == option.value ? "checkmark.circle.fill" : "circle")
                    .font(.title3.weight(.semibold))
                    .foregroundStyle(selectedEndpoint == option.value ? Color(red: 0.04, green: 0.30, blue: 0.57) : Color.secondary)
                    .frame(width: 28)
                VStack(alignment: .leading, spacing: 4) {
                    Text(option.title)
                        .font(.headline)
                        .foregroundStyle(.primary)
                    Text(option.subtitle)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                    Text(option.value)
                        .font(.caption.monospaced())
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .minimumScaleFactor(0.75)
                }
                Spacer(minLength: 0)
            }
            .padding(16)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(
                selectedEndpoint == option.value
                    ? Color(red: 0.93, green: 0.97, blue: 1.0)
                    : Color(red: 0.99, green: 1.0, blue: 0.99),
                in: RoundedRectangle(cornerRadius: 18, style: .continuous)
            )
            .overlay(
                RoundedRectangle(cornerRadius: 18, style: .continuous)
                    .stroke(
                        selectedEndpoint == option.value
                            ? Color(red: 0.30, green: 0.49, blue: 0.79)
                            : Color(red: 0.80, green: 0.86, blue: 0.93),
                        lineWidth: 1
                    )
            )
        }
        .buttonStyle(.plain)
    }

    private var continueButton: some View {
        Button(action: startSelectedWorkspace) {
            HStack {
                if isStarting {
                    ProgressView()
                        .controlSize(.small)
                }
                Text(isStarting ? "Connecting" : "Continue")
                    .font(.headline)
                Spacer()
                Image(systemName: "arrow.right")
                    .font(.headline.weight(.semibold))
            }
            .padding(.horizontal, 18)
            .padding(.vertical, 12)
            .frame(maxWidth: .infinity)
        }
        .buttonStyle(.borderedProminent)
        .controlSize(.large)
        .tint(Color(red: 0.04, green: 0.30, blue: 0.57))
        .disabled(selectedOption == nil || isStarting)
        .accessibilityIdentifier("workspace-selection-continue")
    }

    private var selectedOption: WorkspaceEndpointOption? {
        SettingsRuntime.workspacePresets.first { $0.value == selectedEndpoint }
    }

    private func startSelectedWorkspace() {
        guard let selectedOption else { return }
        isStarting = true
        onContinue(selectedOption)
    }
}

private struct ViantLogoMark: View {
    var body: some View {
        #if canImport(UIKit)
        if let image = viantLogoImage() {
            Image(uiImage: image)
                .resizable()
                .scaledToFit()
                .frame(width: 142)
                .accessibilityLabel("Viant")
        } else {
            fallback
        }
        #else
        fallback
        #endif
    }

    private var fallback: some View {
        Text("VIANT.")
            .font(.system(size: 34, weight: .heavy, design: .serif))
            .foregroundStyle(Color(red: 0.94, green: 0.11, blue: 0.22))
            .accessibilityLabel("Viant")
    }
}

#if canImport(UIKit)
private func viantLogoImage() -> UIImage? {
    guard let url = Bundle.module.url(forResource: "ViantLogo", withExtension: "png"),
          let data = try? Data(contentsOf: url) else {
        return nil
    }
    return UIImage(data: data)
}
#endif

private func workspaceOnboardingBackground() -> some View {
    LinearGradient(
        colors: [
            Color(red: 0.95, green: 0.98, blue: 1.0),
            Color(red: 0.98, green: 0.98, blue: 0.95),
            Color(red: 0.92, green: 0.96, blue: 0.94)
        ],
        startPoint: .topLeading,
        endPoint: .bottomTrailing
    )
}

#Preview {
    WorkspaceSelectionScreen(settingsRuntime: SettingsRuntime()) { _ in }
}
