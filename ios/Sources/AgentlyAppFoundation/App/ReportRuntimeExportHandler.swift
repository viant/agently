import Foundation
import AgentlySDK
import ForgeIOSRuntime

private let reportExportPollIntervalNanoseconds: UInt64 = 1_500_000_000
private let reportExportPollAttempts = 20

typealias SDKJSONValue = AgentlySDK.JSONValue

struct ReportRuntimeExportArtifact: Sendable, Equatable {
    let id: String
    let name: String
    let contentType: String
    let data: Data
}

enum ReportRuntimeExportError: Error, LocalizedError {
    case missingExportRequest
    case emptyToolResult(String)
    case unexpectedToolResult(String)
    case missingJobID
    case failed(String)
    case timedOut
    case missingArtifactID
    case emptyArtifact

    var errorDescription: String? {
        switch self {
        case .missingExportRequest:
            return "No report export request is available."
        case .emptyToolResult(let tool):
            return "Reporting tool \(tool) returned an empty response."
        case .unexpectedToolResult(let tool):
            return "Reporting tool \(tool) returned an unexpected response."
        case .missingJobID:
            return "Report PDF export did not return a job id."
        case .failed(let message):
            return message.isEmpty ? "Report PDF export failed." : message
        case .timedOut:
            return "Report PDF export did not finish in time."
        case .missingArtifactID:
            return "Report PDF export completed without an artifact id."
        case .emptyArtifact:
            return "Report PDF artifact was empty."
        }
    }
}

func registerReportRuntimeExportHandler(
    forgeRuntime: ForgeRuntime,
    client: AgentlyClient,
    conversationIDProvider: @escaping @Sendable () async -> String?,
    onComplete: @escaping @Sendable (ReportRuntimeExportArtifact) async -> Void,
    onError: @escaping @Sendable (String?) async -> Void
) async {
    await forgeRuntime.registerHandler("reportRuntime.exportPdf") { args in
        guard let exportRequest = args.args["exportRequest"]?.objectValue, !exportRequest.isEmpty else {
            await onError(ReportRuntimeExportError.missingExportRequest.localizedDescription)
            return .bool(false)
        }
        do {
            let artifact = try await exportReportRuntimePDF(
                client: client,
                exportRequest: exportRequest,
                conversationID: await conversationIDProvider()?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            )
            await onComplete(artifact)
            await onError(nil)
            return .bool(true)
        } catch {
            await onError(reportRuntimeExportErrorMessage(error))
            return .bool(false)
        }
    }
}

func reportRuntimeExportErrorMessage(_ error: Error) -> String {
    let raw = error.localizedDescription
    let serverDetail: String? = {
        guard let start = raw.firstIndex(of: "{") else { return nil }
        let suffix = String(raw[start...])
        guard let data = suffix.data(using: .utf8),
              let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let message = object["error"] as? String else {
            return nil
        }
        let detail = message
            .replacingOccurrences(of: "\n", with: " ")
            .trimmingCharacters(in: .whitespacesAndNewlines)
        return detail
            .replacingOccurrences(of: "^reporting export:\\s*", with: "", options: .regularExpression)
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .nonEmpty
    }()
    let diagnostic = (serverDetail ?? raw).lowercased()
    if diagnostic.contains("scratchpad") ||
        diagnostic.contains("storage.googleapis.com") ||
        diagnostic.contains("unable to generate access token") {
        return "The PDF was created, but report storage is temporarily unavailable. Please try again."
    }
    guard let serverDetail else {
        return "Unable to create the report PDF. Please try again."
    }
    return "Unable to create the report PDF: \(serverDetail)"
}

func exportReportRuntimePDF(
    client: AgentlyClient,
    exportRequest: [String: ForgeJSONValue],
    conversationID: String = ""
) async throws -> ReportRuntimeExportArtifact {
    let completedJob: ReportExportJobState
    if let fences = exportRequest["fences"]?.arrayValue, !fences.isEmpty {
        let compiled = try await executeReportingToolObject(
            client: client,
            toolName: "reporting:compile_and_export_fenced_report",
            args: [
                "reportId": .string(stringValue(exportRequest["reportId"])),
                "format": .string("pdf"),
                "fences": .array(fences.map(sdkJSONValue(from:))),
                "conversationId": .string(conversationID)
            ],
            conversationID: conversationID
        )
        let job = normalizeReportExportJob(compiled["job"]?.objectValue ?? [:])
        let artifactID = stringValue(compiled["artifact"]?.objectValue?["artifactId"])
        completedJob = ReportExportJobState(
            jobID: job.jobID,
            artifactID: job.artifactID.nonEmpty ?? artifactID,
            artifactRef: job.artifactRef,
            status: job.status,
            error: job.error
        )
    } else {
        let request = normalizeReportRuntimeExportRequest(exportRequest)
        let submitResult = try await executeReportingToolObject(
            client: client,
            toolName: "reporting:submit_export",
            args: request,
            conversationID: conversationID
        )
        completedJob = try await waitForReportExportArtifact(
            client: client,
            initialJob: normalizeReportExportJob(submitResult),
            conversationID: conversationID
        )
    }
    guard !completedJob.artifactID.isEmpty else {
        throw ReportRuntimeExportError.missingArtifactID
    }
    let artifact = try await executeReportingToolObject(
        client: client,
        toolName: "reporting:get_artifact",
        args: [
            "artifactId": .string(completedJob.artifactID),
            "includeData": .bool(true)
        ],
        conversationID: conversationID
    )
    let bytes = decodeReportArtifactBytes(artifact)
    guard !bytes.isEmpty else {
        throw ReportRuntimeExportError.emptyArtifact
    }
    let title = stringValue(exportRequest["title"]).nonEmpty ?? completedJob.artifactRef.nonEmpty ?? completedJob.artifactID
    let name = sanitizeReportRuntimeExportFilename(
        stringValue(artifact["filename"]).nonEmpty
            ?? stringValue(artifact["name"]).nonEmpty
            ?? "\(title).pdf"
    )
    let contentType = stringValue(artifact["contentType"]).nonEmpty ?? "application/pdf"
    return ReportRuntimeExportArtifact(
        id: completedJob.artifactID,
        name: name.isEmpty ? "\(completedJob.artifactID).pdf" : name,
        contentType: contentType,
        data: bytes
    )
}

func downloadReportRuntimeArtifact(
    client: AgentlyClient,
    artifactID: String,
    conversationID: String = ""
) async throws -> ReportRuntimeExportArtifact {
    let normalizedID = artifactID.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !normalizedID.isEmpty else {
        throw ReportRuntimeExportError.missingArtifactID
    }
    let artifact = try await executeReportingToolObject(
        client: client,
        toolName: "reporting:get_artifact",
        args: [
            "artifactId": .string(normalizedID),
            "includeData": .bool(true)
        ],
        conversationID: conversationID
    )
    let bytes = decodeReportArtifactBytes(artifact)
    guard !bytes.isEmpty else {
        throw ReportRuntimeExportError.emptyArtifact
    }
    let name = sanitizeReportRuntimeExportFilename(
        stringValue(artifact["filename"]).nonEmpty
            ?? stringValue(artifact["name"]).nonEmpty
            ?? "(normalizedID).pdf"
    )
    return ReportRuntimeExportArtifact(
        id: normalizedID,
        name: name,
        contentType: stringValue(artifact["contentType"]).nonEmpty ?? "application/pdf",
        data: bytes
    )
}

private func normalizeReportRuntimeExportRequest(_ exportRequest: [String: ForgeJSONValue]) -> [String: SDKJSONValue] {
    let artifactRef = stringValue(exportRequest["artifactRef"]).nonEmpty
        ?? "report://runtime/\(stringValue(exportRequest["title"]).nonEmpty ?? "report")"
    return [
        "artifactRef": .string(artifactRef),
        "format": .string("pdf"),
        "scope": .string("draft"),
        "reportSpec": sdkJSONValue(from: exportRequest["reportSpec"] ?? ForgeJSONValue.null),
        "reportFill": sdkJSONValue(from: exportRequest["reportFill"] ?? ForgeJSONValue.null),
        "reportPrint": sdkJSONValue(from: exportRequest["reportPrint"] ?? ForgeJSONValue.null)
    ]
}

private func waitForReportExportArtifact(
    client: AgentlyClient,
    initialJob: ReportExportJobState,
    conversationID: String
) async throws -> ReportExportJobState {
    var job = initialJob
    for attempt in 0..<reportExportPollAttempts {
        if !job.artifactID.isEmpty {
            return job
        }
        if ["failed", "canceled", "cancelled"].contains(job.status.lowercased()) {
            throw ReportRuntimeExportError.failed(job.error)
        }
        if job.jobID.isEmpty {
            throw ReportRuntimeExportError.missingJobID
        }
        if attempt > 0 {
            try await Task.sleep(nanoseconds: reportExportPollIntervalNanoseconds)
        }
        let statusResult = try await executeReportingToolObject(
            client: client,
            toolName: "reporting:get_export_status",
            args: ["jobId": .string(job.jobID)],
            conversationID: conversationID
        )
        job = normalizeReportExportJob(statusResult)
    }
    throw ReportRuntimeExportError.timedOut
}

private struct ReportExportJobState: Sendable, Equatable {
    let jobID: String
    let artifactID: String
    let artifactRef: String
    let status: String
    let error: String
}

private func normalizeReportExportJob(_ value: [String: SDKJSONValue]) -> ReportExportJobState {
    ReportExportJobState(
        jobID: stringValue(value["jobId"]),
        artifactID: stringValue(value["artifactId"]),
        artifactRef: stringValue(value["artifactRef"]),
        status: stringValue(value["status"]),
        error: stringValue(value["error"])
    )
}

private func executeReportingToolObject(
    client: AgentlyClient,
    toolName: String,
    args: [String: SDKJSONValue],
    conversationID: String
) async throws -> [String: SDKJSONValue] {
    let raw = try await client.executeTool(
        name: toolName,
        args: args,
        conversationID: conversationID.isEmpty ? nil : conversationID
    )
    guard !raw.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
        throw ReportRuntimeExportError.emptyToolResult(toolName)
    }
    let value = try JSONDecoder().decode(SDKJSONValue.self, from: Data(raw.utf8))
    guard case .object(let object) = value else {
        throw ReportRuntimeExportError.unexpectedToolResult(toolName)
    }
    return object
}

private func decodeReportArtifactBytes(_ artifact: [String: SDKJSONValue]) -> Data {
    let data = stringValue(artifact["data"])
    if !data.isEmpty, let decoded = Data(base64Encoded: data) {
        return decoded
    }
    guard case .array(let bytes)? = artifact["bytes"] else {
        return Data()
    }
    return Data(bytes.compactMap { value in
        guard case .number(let number) = value else { return nil }
        return UInt8(exactly: Int(number))
    })
}

private func sdkJSONValue(from value: ForgeJSONValue) -> SDKJSONValue {
    switch value {
    case .string(let string):
        return .string(string)
    case .number(let number):
        return .number(number)
    case .bool(let bool):
        return .bool(bool)
    case .array(let values):
        return .array(values.map(sdkJSONValue(from:)))
    case .object(let object):
        return .object(object.mapValues(sdkJSONValue(from:)))
    case .null:
        return .null
    }
}

private func stringValue(_ value: ForgeJSONValue?) -> String {
    guard let value else { return "" }
    switch value {
    case .string(let string):
        return string.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines)
    case .number(let number):
        return String(number)
    case .bool(let bool):
        return bool ? "true" : "false"
    default:
        return ""
    }
}

private func stringValue(_ value: SDKJSONValue?) -> String {
    guard let value else { return "" }
    switch value {
    case .string(let string):
        return string.trimmingCharacters(in: .whitespacesAndNewlines)
    case .number(let number):
        return String(number)
    case .bool(let bool):
        return bool ? "true" : "false"
    default:
        return ""
    }
}

private func sanitizeReportRuntimeExportFilename(_ value: String) -> String {
    let base = value.trimmingCharacters(in: .whitespacesAndNewlines).nonEmpty ?? "report.pdf"
    let cleaned = base
        .replacingOccurrences(of: "/", with: "-")
        .replacingOccurrences(of: ":", with: "-")
    return cleaned.contains(".") ? cleaned : "\(cleaned).pdf"
}

private extension String {
    var nonEmpty: String? {
        let trimmed = trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nil : trimmed
    }
}
