import SwiftUI

public struct SettingsScreen: View {
    @ObservedObject private var runtime: SettingsRuntime
    let workspaceRoot: String?
    let workspaceDefaultAgentID: String?
    let availableAgents: [WorkspaceAgentOption]
    let agentAutoSelectionEnabled: Bool
    let oauthProviderLabels: [String]
    let oauthScopes: [String]
    let onApply: () -> Void

    public init(
        runtime: SettingsRuntime,
        workspaceRoot: String? = nil,
        workspaceDefaultAgentID: String? = nil,
        availableAgents: [WorkspaceAgentOption] = [],
        agentAutoSelectionEnabled: Bool = false,
        oauthProviderLabels: [String] = [],
        oauthScopes: [String] = [],
        onApply: @escaping () -> Void = {}
    ) {
        self.runtime = runtime
        self.workspaceRoot = workspaceRoot
        self.workspaceDefaultAgentID = workspaceDefaultAgentID
        self.availableAgents = availableAgents
        self.agentAutoSelectionEnabled = agentAutoSelectionEnabled
        self.oauthProviderLabels = oauthProviderLabels
        self.oauthScopes = oauthScopes
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
                if !hasWorkspaceMetadata {
                    Text("Workspace and agent options appear after the app successfully loads metadata from this URL.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
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
            if hasWorkspaceMetadata {
                Section("Workspace") {
                if let workspaceRoot, !workspaceRoot.isEmpty {
                    LabeledContent("Workspace", value: workspaceRoot)
                        .font(.footnote)
                }
                LabeledContent(
                    "Workspace Default Agent",
                    value: workspaceDefaultAgentID?.isEmpty == false ? workspaceDefaultAgentID! : "n/a"
                )
                if agentAutoSelectionEnabled {
                    Text("This workspace supports agent auto-selection. Leaving the app preference empty will use workspace default/auto behavior.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
                Picker("Agent Preference", selection: $runtime.preferredAgentID) {
                    Text(agentAutoSelectionEnabled ? "Auto / Workspace Default" : "Workspace Default")
                        .tag("")
                    ForEach(availableAgents, id: \.id) { agent in
                        Text(agent.displayName).tag(agent.id)
                    }
                }
                #if os(iOS)
                .pickerStyle(.navigationLink)
                #endif
                if let selected = selectedAgentLabel {
                    LabeledContent("Current Selection", value: selected)
                        .font(.footnote)
                }
            }
            }
            if hasOAuthConfiguration {
                Section("OAuth") {
                    LabeledContent("Providers", value: oauthProviderLabels.joined(separator: ", "))
                        .font(.footnote)
                    if oauthScopes.isEmpty {
                        Text("The workspace is OAuth-capable, but it did not advertise scopes.")
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    } else {
                        VStack(alignment: .leading, spacing: 8) {
                            Text("Scopes")
                                .font(.footnote.weight(.semibold))
                            ScrollView(.horizontal, showsIndicators: false) {
                                HStack(spacing: 8) {
                                    ForEach(oauthScopes, id: \.self) { scope in
                                        Text(scope)
                                            .font(.caption.weight(.medium))
                                            .padding(.horizontal, 10)
                                            .padding(.vertical, 6)
                                            .background(Capsule().fill(Color.secondary.opacity(0.12)))
                                    }
                                }
                            }
                        }
                    }
                    Text("OAuth client details are currently workspace-driven on iOS. This screen reflects what the workspace advertises; it does not yet override client configuration locally.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
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

    private var hasWorkspaceMetadata: Bool {
        let rootLoaded = workspaceRoot?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false
        let defaultLoaded = workspaceDefaultAgentID?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false
        return rootLoaded || defaultLoaded || !availableAgents.isEmpty
    }

    private var hasOAuthConfiguration: Bool {
        !oauthProviderLabels.isEmpty || !oauthScopes.isEmpty
    }

    private var selectedAgentLabel: String? {
        let preferred = runtime.preferredAgentID.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !preferred.isEmpty else {
            if agentAutoSelectionEnabled {
                return "Auto / Workspace Default"
            }
            return workspaceDefaultAgentID?.isEmpty == false ? workspaceDefaultAgentID! : "Workspace Default"
        }
        return availableAgents.first(where: { $0.id == preferred })?.displayName ?? preferred
    }
}
