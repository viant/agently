import SwiftUI
import AgentlySDK
import ForgeIOSRuntime
import ForgeIOSUI

struct AutomationWorkspaceScreen: View {
    let forgeRuntime: ForgeRuntime
    let client: AgentlyClient
    let onOpenConversation: (String) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var window: ForgeRuntime.WindowState?
    @State private var windowContext: WindowContext?
    @State private var errorMessage: String?
    @State private var historyContext: AutomationHistoryContext?

    var body: some View {
        NavigationStack {
            Group {
                if let window, let windowContext, let metadata = window.metadata {
                    WindowContentView(
                        runtime: forgeRuntime,
                        window: windowContext,
                        metadata: metadata
                    )
                    .environment(\.forgePresentationDensity, .compact)
                } else if let errorMessage {
                    ContentUnavailableView(
                        "Automation unavailable",
                        systemImage: "clock.badge.exclamationmark",
                        description: Text(errorMessage)
                    )
                } else {
                    ProgressView("Loading automation…")
                }
            }
            .navigationTitle("Automation")
            .modifier(AutomationInlineNavigationTitleDisplayMode())
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Close") { dismiss() }
                }
            }
        }
        .sheet(item: $historyContext) { context in
            AutomationRunHistoryView(
                client: client,
                context: context,
                onOpenConversation: { conversationID in
                    historyContext = nil
                    onOpenConversation(conversationID)
                }
            )
        }
        .task { await openAutomationWindow() }
    }

    @MainActor
    private func openAutomationWindow() async {
        do {
            await registerAutomationHandlers()
            let loader = makeForgeAgentlyWindowMetadataLoader(
                client: client,
                targetContext: forgeRuntime.targetContext
            )
            guard let metadata = try await loader(
                ForgeRuntime.WindowMetadataRequest(windowID: "schedule", windowKey: "schedule")
            ) else {
                errorMessage = "Automation metadata could not be loaded."
                return
            }
            let resolved = await forgeRuntime.openWindowInline(
                key: "schedule",
                title: "Automation",
                metadata: metadata
            )
            window = resolved
            windowContext = await forgeRuntime.windowContext(id: resolved.id)
            errorMessage = nil
        } catch {
            errorMessage = automationMetadataErrorDescription(error)
        }
    }

    private func registerAutomationHandlers() async {
        await forgeRuntime.registerHandler("schedule.noListSelection") { args in
            .bool(!(await automationHasSelection(runtime: forgeRuntime, args: args)))
        }
        await forgeRuntime.registerHandler("schedule.hasListSelection") { args in
            .bool(await automationHasSelection(runtime: forgeRuntime, args: args))
        }
        await forgeRuntime.registerHandler("schedule.addNewSchedule") { args in
            guard let windowID = args.context?.windowID else { return .bool(false) }
            let dataSourceRef = args.context?.dataSourceRef.isEmpty == false ? args.context!.dataSourceRef : "schedules"
            await forgeRuntime.setDataSourceForm(
                windowID: windowID,
                dataSourceRef: dataSourceRef,
                values: [
                    "scheduleEditorKind": .string("calendar"),
                    "calendarPattern": .string("once"),
                    "calendarTime": .string("09:00 AM"),
                    "weekdays": .array([]),
                    "visibility": .string("private"),
                    "timezone": .string("UTC"),
                    "timeoutSeconds": .number(300),
                    "enabled": .bool(false)
                ]
            )
            await forgeRuntime.setWindowFormValue(
                windowID: windowID,
                values: ["automationView": .string("editor")]
            )
            return .bool(true)
        }
        await forgeRuntime.registerHandler("schedule.backToList") { args in
            guard let windowID = args.context?.windowID else { return .bool(false) }
            await forgeRuntime.setWindowFormValue(
                windowID: windowID,
                values: ["automationView": .string("list")]
            )
            return .bool(true)
        }
        await forgeRuntime.registerHandler("schedule.editSelected") { args in
            guard let context = args.context,
                  let selected = await forgeRuntime.dataSourceSelectionState(
                    windowID: context.windowID,
                    dataSourceRef: context.dataSourceRef.isEmpty ? "schedules" : context.dataSourceRef
                  ).selected,
                  selected["id"]?.stringValue?.isEmpty == false else { return .bool(false) }
            let ref = context.dataSourceRef.isEmpty ? "schedules" : context.dataSourceRef
            await forgeRuntime.setDataSourceForm(windowID: context.windowID, dataSourceRef: ref, values: selected)
            await forgeRuntime.setWindowFormValue(
                windowID: context.windowID,
                values: ["automationView": .string("editor")]
            )
            return .bool(true)
        }
        await forgeRuntime.registerHandler("schedule.syncScheduleFields") { _ in .bool(true) }
        await forgeRuntime.registerHandler("schedule.showIfCalendar") { args in
            .bool(await automationEditorKind(runtime: forgeRuntime, args: args) == "calendar")
        }
        await forgeRuntime.registerHandler("schedule.showIfCalendarEvery") { args in
            let form = await automationForm(runtime: forgeRuntime, args: args)
            return .bool(
                automationEditorKind(form: form) == "calendar" && form["calendarPattern"]?.stringValue == "every"
            )
        }
        await forgeRuntime.registerHandler("schedule.showIfElapsed") { args in
            .bool(await automationEditorKind(runtime: forgeRuntime, args: args) == "elapsed")
        }
        await forgeRuntime.registerHandler("schedule.showIfAdvanced") { args in
            .bool(await automationEditorKind(runtime: forgeRuntime, args: args) == "advanced")
        }
        await forgeRuntime.registerHandler("schedule.showIfEdit") { args in
            .bool(!(await automationForm(runtime: forgeRuntime, args: args)["id"]?.stringValue ?? "").isEmpty)
        }
        await forgeRuntime.registerHandler("schedule.hideField") { _ in .bool(false) }
        await forgeRuntime.registerHandler("schedule.onFetchSchedules") { _ in .bool(true) }
        await forgeRuntime.registerHandler("schedule.onFetchAgentsLov") { _ in .bool(true) }
        await forgeRuntime.registerHandler("schedule.onFetchModelsLov") { _ in .bool(true) }
        await forgeRuntime.registerHandler("schedule.applyLookupFilter") { _ in .bool(true) }
        await forgeRuntime.registerHandler("dataSource.isFormDirty") { _ in .bool(true) }
        await forgeRuntime.registerHandler("schedule.saveSchedule") { args in
            guard let context = args.context else { return .bool(false) }
            let ref = context.dataSourceRef.isEmpty ? "schedules" : context.dataSourceRef
            let form = await forgeRuntime.formJSONValue(windowID: context.windowID, dataSourceRef: ref)
            guard let schedule = automationSchedule(from: form) else { return .bool(false) }
            do {
                try await client.upsertSchedules([schedule])
                await forgeRuntime.refreshDataSourceCollection(windowID: context.windowID, dataSourceRef: ref)
                await forgeRuntime.setWindowFormValue(
                    windowID: context.windowID,
                    values: ["automationView": .string("list")]
                )
                return .bool(true)
            } catch {
                return .bool(false)
            }
        }
        await forgeRuntime.registerHandler("schedule.runSelected") { args in
            guard let id = await automationSelectedScheduleID(runtime: forgeRuntime, args: args) else {
                return .bool(false)
            }
            do {
                try await client.runScheduleNow(id: id)
                return .bool(true)
            } catch {
                return .bool(false)
            }
        }
        await forgeRuntime.registerHandler("schedule.openHistory") { args in
            guard let context = args.context else { return .bool(false) }
            let ref = context.dataSourceRef.isEmpty ? "schedules" : context.dataSourceRef
            guard let selected = await forgeRuntime.dataSourceSelectionState(
                windowID: context.windowID,
                dataSourceRef: ref
            ).selected,
                  let id = selected["id"]?.stringValue,
                  !id.isEmpty else { return .bool(false) }
            let name = selected["name"]?.stringValue?.nonEmpty ?? "Automation"
            await MainActor.run {
                historyContext = AutomationHistoryContext(scheduleID: id, scheduleName: name)
            }
            return .bool(true)
        }
        await forgeRuntime.registerHandler("schedule.deleteSchedule") { args in
            guard let context = args.context,
                  let id = await automationSelectedScheduleID(runtime: forgeRuntime, args: args) else {
                return .bool(false)
            }
            do {
                try await client.deleteSchedule(id: id)
                await forgeRuntime.refreshDataSourceCollection(
                    windowID: context.windowID,
                    dataSourceRef: context.dataSourceRef.isEmpty ? "schedules" : context.dataSourceRef
                )
                return .bool(true)
            } catch {
                return .bool(false)
            }
        }
    }
}

private struct AutomationInlineNavigationTitleDisplayMode: ViewModifier {
    @ViewBuilder
    func body(content: Content) -> some View {
        #if os(iOS)
        content.navigationBarTitleDisplayMode(.inline)
        #else
        content
        #endif
    }
}

private func automationHasSelection(runtime: ForgeRuntime, args: ExecutionArgs) async -> Bool {
    await automationSelectedScheduleID(runtime: runtime, args: args) != nil
}

private func automationSelectedScheduleID(runtime: ForgeRuntime, args: ExecutionArgs) async -> String? {
    guard let context = args.context else { return nil }
    let selection = await runtime.dataSourceSelectionState(
        windowID: context.windowID,
        dataSourceRef: context.dataSourceRef.isEmpty ? "schedules" : context.dataSourceRef
    )
    return selection.selected?["id"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines).nonEmpty
}

private func automationForm(runtime: ForgeRuntime, args: ExecutionArgs) async -> [String: ForgeIOSRuntime.JSONValue] {
    guard let context = args.context else { return [:] }
    return await runtime.formJSONValue(
        windowID: context.windowID,
        dataSourceRef: context.dataSourceRef.isEmpty ? "schedules" : context.dataSourceRef
    )
}

private func automationEditorKind(runtime: ForgeRuntime, args: ExecutionArgs) async -> String {
    automationEditorKind(form: await automationForm(runtime: runtime, args: args))
}

private func automationEditorKind(form: [String: ForgeIOSRuntime.JSONValue]) -> String {
    let value = form["scheduleEditorKind"]?.stringValue?.lowercased() ?? ""
    return ["calendar", "elapsed", "advanced"].contains(value) ? value : "calendar"
}

private func automationSchedule(from form: [String: ForgeIOSRuntime.JSONValue]) -> Schedule? {
    let name = form["name"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    let agentRef = form["agentRef"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    let taskPrompt = form["taskPrompt"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    guard !name.isEmpty, !agentRef.isEmpty, !taskPrompt.isEmpty else { return nil }
    let kind = automationEditorKind(form: form)
    let intervalValue = max(1, form["elapsedIntervalValue"]?.intValue ?? 1)
    let intervalUnit = form["elapsedIntervalUnit"]?.stringValue ?? "hours"
    let multiplier = intervalUnit == "minutes" ? 60 : (intervalUnit == "days" ? 86_400 : 3_600)
    let intervalSeconds = kind == "elapsed" ? intervalValue * multiplier : nil
    let cronExpression: String? = {
        if kind == "advanced" {
            return form["cronExpr"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines)
        }
        guard kind == "calendar" else { return nil }
        let time = form["calendarTime"]?.stringValue ?? "09:00 AM"
        let components = automationTimeComponents(time)
        let weekdays = (form["weekdays"]?.arrayValue ?? []).compactMap(\.stringValue)
        let weekdayNumbers = weekdays.compactMap { ["sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6][$0] }
        let dayField = weekdayNumbers.isEmpty ? "*" : weekdayNumbers.map(String.init).joined(separator: ",")
        return "\(components.minute) \(components.hour) * * \(dayField)"
    }()
    return Schedule(
        id: form["id"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines).nonEmpty ?? UUID().uuidString,
        name: name,
        description: form["description"]?.stringValue,
        visibility: form["visibility"]?.stringValue ?? "private",
        agentRef: agentRef,
        modelOverride: form["modelOverride"]?.stringValue,
        enabled: form["enabled"]?.boolValue ?? false,
        startAt: form["startAt"]?.stringValue,
        endAt: form["endAt"]?.stringValue,
        scheduleType: kind == "elapsed" ? "interval" : "cron",
        cronExpr: cronExpression,
        intervalSeconds: intervalSeconds,
        timezone: form["timezone"]?.stringValue ?? "UTC",
        timeoutSeconds: form["timeoutSeconds"]?.intValue ?? 300,
        taskPromptURI: form["taskPromptUri"]?.stringValue,
        taskPrompt: taskPrompt
    )
}

private func automationTimeComponents(_ value: String) -> (hour: Int, minute: Int) {
    let formats = ["h:mm a", "HH:mm"]
    for format in formats {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.dateFormat = format
        if let date = formatter.date(from: value.trimmingCharacters(in: .whitespacesAndNewlines)) {
            let values = Calendar(identifier: .gregorian).dateComponents([.hour, .minute], from: date)
            return (values.hour ?? 9, values.minute ?? 0)
        }
    }
    return (9, 0)
}

private struct AutomationHistoryContext: Identifiable {
    let scheduleID: String
    let scheduleName: String
    var id: String { scheduleID }
}

private struct AutomationRunHistoryView: View {
    let client: AgentlyClient
    let context: AutomationHistoryContext
    let onOpenConversation: (String) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var runs: [ScheduleRun] = []
    @State private var isLoading = true
    @State private var errorMessage: String?

    var body: some View {
        NavigationStack {
            Group {
                if isLoading {
                    ProgressView("Loading runs…")
                } else if let errorMessage {
                    ContentUnavailableView(
                        "Run history unavailable",
                        systemImage: "clock.badge.exclamationmark",
                        description: Text(errorMessage)
                    )
                } else if runs.isEmpty {
                    ContentUnavailableView(
                        "No runs yet",
                        systemImage: "clock.arrow.circlepath",
                        description: Text("Run this automation manually or wait for its next scheduled time.")
                    )
                } else {
                    List {
                        Section {
                            Text(context.scheduleName)
                                .font(.subheadline.weight(.semibold))
                        }
                        ForEach(runs) { run in
                        HStack(spacing: 12) {
                            VStack(alignment: .leading, spacing: 4) {
                                Text(run.status?.capitalized ?? "Run")
                                    .font(.headline)
                                Text(run.completedAt ?? run.startedAt ?? run.createdAt ?? "Recent")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                                if let message = run.errorMessage?.trimmingCharacters(in: .whitespacesAndNewlines), !message.isEmpty {
                                    Text(message)
                                        .font(.caption)
                                        .foregroundStyle(.red)
                                }
                            }
                            Spacer()
                            if let conversationID = run.conversationID?.trimmingCharacters(in: .whitespacesAndNewlines), !conversationID.isEmpty {
                                Button {
                                    onOpenConversation(conversationID)
                                } label: {
                                    Image(systemName: "eye.fill")
                                        .frame(width: 34, height: 34)
                                        .background(Color.blue.opacity(0.12), in: Circle())
                                }
                                .buttonStyle(.plain)
                                .accessibilityLabel("Open conversation")
                            }
                        }
                        }
                    }
                    .refreshable { await load() }
                }
            }
            .navigationTitle("Run History")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Close") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button { Task { await load() } } label: {
                        Image(systemName: "arrow.clockwise")
                    }
                    .accessibilityLabel("Refresh run history")
                }
            }
        }
        .task { await load() }
    }

    @MainActor
    private func load() async {
        isLoading = true
        defer { isLoading = false }
        do {
            runs = try await client.listScheduleRuns(scheduleID: context.scheduleID, page: 1, size: 50).rows
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}

private extension String {
    var nonEmpty: String? { isEmpty ? nil : self }
}

private func automationMetadataErrorDescription(_ error: Error) -> String {
    switch error {
    case DecodingError.keyNotFound(let key, let context):
        let path = context.codingPath.map(\.stringValue).joined(separator: ".")
        return "Missing \(key.stringValue) at \(path.isEmpty ? "metadata root" : path)."
    case DecodingError.typeMismatch(let type, let context):
        let path = context.codingPath.map(\.stringValue).joined(separator: ".")
        return "Expected \(type) at \(path.isEmpty ? "metadata root" : path)."
    case DecodingError.valueNotFound(let type, let context):
        let path = context.codingPath.map(\.stringValue).joined(separator: ".")
        return "Missing \(type) value at \(path.isEmpty ? "metadata root" : path)."
    case DecodingError.dataCorrupted(let context):
        let path = context.codingPath.map(\.stringValue).joined(separator: ".")
        return "Invalid metadata at \(path.isEmpty ? "metadata root" : path): \(context.debugDescription)"
    default:
        return error.localizedDescription
    }
}
