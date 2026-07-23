import SwiftUI
import ForgeIOSRuntime
import ForgeIOSUI
import AgentlySDK

typealias TranscriptContentPart = TranscriptEnvelopePart
typealias ForgeUIPayload = TranscriptForgeUIPayload
typealias MaterializedForgeDataBlock = TranscriptForgeDataStore

struct TranscriptMessageContent: View {
    let markdown: String
    let renderedParts: [TranscriptCanonicalPart]?
    let renderedReports: [TranscriptCanonicalReport]?
	let client: AgentlyClient?

    var body: some View {
        let sourceParts = renderedParts
            .map(TranscriptEnvelope.fromCanonical)
            ?? parseTranscriptContentParts(markdown)
        let parts = renderedReports?.isEmpty == false
            ? sourceParts.map { part in
                if case .markdown(let text) = part {
                    return .markdown(TranscriptEnvelope.suppressProgressiveTransport(in: text))
                }
                return part
            }
            : sourceParts
        VStack(alignment: .leading, spacing: 10) {
            ForEach(Array(parts.enumerated()), id: \.offset) { _, part in
                switch part {
                case .markdown(let text):
                    if !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                        MarkdownRenderer(markdown: text).textSelection(.enabled)
                    }
                case .forgeUI(let payload, let dataStore):
                    TranscriptForgeUIView(payload: payload, dataStore: dataStore)
                }
            }
            ForEach(renderedReports ?? [], id: \.stableIdentity) { report in
                TranscriptInlineReportView(report: report, client: client)
            }
        }
    }
}

private extension TranscriptCanonicalReport {
    var stableIdentity: String {
        "\(scope):\(id):\(resetVersion)"
    }

    var refreshIdentity: String {
        "\(stableIdentity):\(sequence ?? 0):\(status)"
    }
}

private struct TranscriptInlineReportView: View {
    let report: TranscriptCanonicalReport
	let client: AgentlyClient?

    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    @State private var metadata: WindowMetadata?
    @State private var windowContext: WindowContext?
    @State private var errorMessage: String?
    @State private var runtime = ForgeRuntime()

    var body: some View {
        Group {
            if let metadata, let windowContext {
                WindowContentView(
                    runtime: runtime,
                    window: windowContext,
                    metadata: metadata,
                    scrollEnabled: true,
                    contentPadding: 4
                )
                .environment(\.forgePresentationDensity, .compact)
                .frame(maxHeight: TranscriptInlinePresentationPolicy.resolve(
                    metadata: metadata,
                    horizontalSizeClass: horizontalSizeClass
                ).maximumHeight)
                .clipped()
            } else {
                VStack(alignment: .leading, spacing: 6) {
                    Text(report.id).font(.headline)
                    Text(errorMessage ?? "Loading report…")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(12)
                .background(Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 12))
            }
        }
        .task(id: report.refreshIdentity) {
            do {
				let hydrated = try await hydrateInlineReport(report, client: client)
                let artifact = try InlineReportRuntimeCompiler.compile(hydrated)
                let state = await runtime.openWindowInline(
                    key: "inline-report-\(report.scope)-\(report.id)",
                    title: artifact.reportSpec.objectValue?["title"]?.stringValue ?? report.id,
                    metadata: artifact.metadata
                )
                metadata = artifact.metadata
                windowContext = await runtime.windowContext(id: state.id)
                errorMessage = nil
            } catch {
                metadata = nil
                windowContext = nil
                errorMessage = error.localizedDescription
            }
        }
    }
}

private func hydrateInlineReport(
	_ report: TranscriptCanonicalReport,
	client: AgentlyClient?
) async throws -> TranscriptCanonicalReport {
	let requests = InlineReportRuntimeCompiler.workspaceDatasetRequests(report)
	guard !requests.isEmpty, let client else { return report }
	var dataSources = report.dataSources
	for request in requests {
		let inputData = try JSONEncoder().encode(request.inputs)
		let inputs = try JSONDecoder().decode([String: AgentlySDK.JSONValue].self, from: inputData)
		let output = try await client.fetchDatasource(FetchDatasourceInput(
			id: request.dataSourceRef,
			inputs: inputs
		))
		let rowsData = try JSONEncoder().encode(output.rows)
		let rows = try JSONDecoder().decode(ForgeIOSRuntime.JSONValue.self, from: rowsData)
		dataSources[request.id] = TranscriptCanonicalData(
			version: 2,
			reportRef: report.id,
			id: request.id,
			format: "json",
			mode: "replace",
			payload: rows
		)
	}
	return TranscriptCanonicalReport(
		scope: report.scope,
		id: report.id,
		grammar: report.grammar,
		status: report.status,
		sequence: report.sequence,
		resetVersion: report.resetVersion,
		source: report.source,
		dataSources: dataSources
	)
}

private struct TranscriptForgeUIView: View {
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass

    let payload: ForgeUIPayload
    let dataStore: [String: MaterializedForgeDataBlock]

    @State private var windowContext: WindowContext?
    @State private var windowID: String?
    @State private var presentationError: String?
    @State private var runtime = ForgeRuntime()

    var body: some View {
        let presentation = try? TranscriptWindowBuilder.presentation(payload: payload, dataStore: dataStore)
        Group {
            if let presentation, let windowContext {
                WindowContentView(
                    runtime: runtime,
                    window: windowContext,
                    metadata: presentation.metadata,
                    scrollEnabled: true,
                    contentPadding: 4
                )
                .environment(\.forgePresentationDensity, .compact)
                .frame(maxHeight: TranscriptInlinePresentationPolicy.resolve(
                    metadata: presentation.metadata,
                    horizontalSizeClass: horizontalSizeClass
                ).maximumHeight)
                .clipped()
            } else if let presentationError {
                unavailableView(message: presentationError)
            } else {
                loadingView
            }
        }
        .task(id: renderTaskKey(for: presentation?.dataStore ?? dataStore)) {
            presentationError = nil
            guard let presentation else {
                windowContext = nil
                presentationError = "This interactive response has no supported Forge content."
                return
            }
            await openInlineWindow(presentation)
        }
    }

    private var loadingView: some View {
        unavailableView(message: "Loading interactive content…")
    }

    private func unavailableView(message: String) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            if let title = payload.title, !title.isEmpty {
                Text(title).font(.headline)
            }
            Text(message)
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(12)
        .background(Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 12))
    }

    private func renderTaskKey(for store: [String: MaterializedForgeDataBlock]) -> String {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        let dataFingerprint = store.keys.sorted().map { key in
            let rows = store[key]?.rows ?? .null
            let encoded = (try? encoder.encode(rows)).flatMap { String(data: $0, encoding: .utf8) } ?? "null"
            return "\(key):\(encoded)"
        }.joined(separator: "|")
        let payloadFingerprint = (try? encoder.encode(payload)).flatMap { String(data: $0, encoding: .utf8) } ?? "{}"
        return "\(payloadFingerprint):\(dataFingerprint)"
    }

    private func openInlineWindow(_ presentation: TranscriptWindowPresentation) async {
        let title = payload.title ?? "Forge content"
        let state: ForgeRuntime.WindowState
        if let windowID,
           let updated = await runtime.updateWindowInline(id: windowID, title: title, metadata: presentation.metadata) {
            state = updated
        } else {
            state = await runtime.openWindowInline(
                key: "transcript-\(UUID().uuidString)",
                title: title,
                metadata: presentation.metadata
            )
        }
        windowID = state.id
        await hydrateTranscriptDataSources(windowID: state.id, dataStore: presentation.dataStore)
        windowContext = await runtime.windowContext(id: state.id)
    }

    private func hydrateTranscriptDataSources(windowID: String, dataStore: [String: MaterializedForgeDataBlock]) async {
        for (dataSourceRef, block) in dataStore {
            let rows = TranscriptEnvelope.rows(from: block.rows)
            await runtime.setDataSourceCollection(windowID: windowID, dataSourceRef: dataSourceRef, rows: rows)
            if rows.isEmpty {
                await runtime.setDataSourceMetrics(windowID: windowID, dataSourceRef: dataSourceRef, values: [:])
                await runtime.setDataSourceSelection(windowID: windowID, dataSourceRef: dataSourceRef, selected: nil)
                await runtime.setDataSourceForm(windowID: windowID, dataSourceRef: dataSourceRef, values: [:])
            } else if rows.count == 1, let first = rows.first {
                await runtime.setDataSourceMetrics(windowID: windowID, dataSourceRef: dataSourceRef, values: first)
            }
        }
    }
}

func parseTranscriptContentParts(_ markdown: String) -> [TranscriptContentPart] {
    TranscriptEnvelope.parse(markdown)
}


/// Compatibility seam for app tests. Generic adaptation belongs to Forge.
func buildTranscriptForgeWindowMetadata(
    payload: ForgeUIPayload,
    dataStore: [String: MaterializedForgeDataBlock]
) throws -> WindowMetadata {
    try TranscriptWindowBuilder.presentation(payload: payload, dataStore: dataStore).metadata
}
