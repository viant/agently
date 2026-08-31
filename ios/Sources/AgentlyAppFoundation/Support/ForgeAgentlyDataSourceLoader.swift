import Foundation
import AgentlySDK
import ForgeIOSRuntime

func makeForgeAgentlyDataSourceLoader(
    client: AgentlyClient,
    conversationIDProvider: @escaping @Sendable () async -> String? = { nil }
) -> @Sendable (ForgeRuntime.DataSourceFetchRequest) async throws -> ForgeRuntime.DataSourceFetchResult? {
    return { request in
        let service = request.dataSource.service
        let endpoint = service?.endpoint?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard endpoint == "agentlyAPI" else {
            return nil
        }
        let uri = service?.uri?.trimmingCharacters(in: .whitespacesAndNewlines)
            ?? request.dataSource.uri?.trimmingCharacters(in: .whitespacesAndNewlines)
            ?? ""
        let normalizedURI = uri.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        if normalizedURI == "v1/api/agently/scheduler" {
            return try await loadSchedules(client: client, request: request)
        }
        if normalizedURI == "v1/api/agently/scheduler/run" {
            return try await loadScheduleRuns(client: client, request: request)
        }
        if normalizedURI == "v1/workspace/metadata" {
            return try await loadWorkspaceOptions(client: client, request: request)
        }
        if normalizedURI == "v1/workspace/metadata/publicagents" {
            return try await loadPublicAgents(client: client)
        }
        guard let datasourceID = extractDatasourceID(from: uri) else {
            return nil
        }

        var inputs = request.resolvedInputs
        if let nestedInput = request.input.parameters["input"] {
            inputs["input"] = nestedInput
        }
        if let page = request.input.parameters["page"] {
            inputs["page"] = page
        }
        for (key, value) in request.input.parameters where key != "input" && key != "page" && key != "parameters" {
            if inputs[key] == nil {
                inputs[key] = value
            }
        }
        if !request.input.filter.isEmpty {
            if inputs["input"]?.objectValue != nil {
                var inputObject = inputs["input"]?.objectValue ?? [:]
                var queryObject = inputObject["query"]?.objectValue ?? [:]
                queryObject.merge(request.input.filter) { _, new in new }
                inputObject["query"] = .object(queryObject)
                inputs["input"] = .object(inputObject)
            } else {
                inputs.merge(request.input.filter) { _, new in new }
            }
        }
        if let page = request.input.page {
            inputs["page"] = .number(Double(page))
        }
        if let paging = request.dataSource.paging, paging.enabled != false {
            let pageKey = paging.parameters["page"]?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            let sizeKey = paging.parameters["size"]?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            if !pageKey.isEmpty, inputs[pageKey] == nil {
                inputs[pageKey] = .number(Double(request.input.page ?? 1))
            }
            if !sizeKey.isEmpty, inputs[sizeKey] == nil, let size = paging.size, size > 0 {
                inputs[sizeKey] = .number(Double(size))
            }
        }

        let requestConversationID = request.conversationID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let fallbackConversationID = requestConversationID.isEmpty
            ? await conversationIDProvider()?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            : ""
        let conversationID = requestConversationID.isEmpty ? fallbackConversationID : requestConversationID
        let response = try await client.fetchDatasource(
            FetchDatasourceInput(
                id: datasourceID,
                inputs: inputs.isEmpty ? nil : inputs.mapValues(\.appValue),
                conversationId: conversationID.isEmpty ? nil : conversationID
            )
        )
        return ForgeRuntime.DataSourceFetchResult(
            rows: response.rows.map { $0.mapValues(\.forgeValue) },
            metrics: response.metrics?.mapValues(\.forgeValue) ?? [:]
        )
    }
}

private func loadSchedules(
    client: AgentlyClient,
    request: ForgeRuntime.DataSourceFetchRequest
) async throws -> ForgeRuntime.DataSourceFetchResult {
    let query = request.input.filter["name"]?.stringValue?
        .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    let schedules = try await client.listSchedules().filter {
        query.isEmpty || $0.name.localizedCaseInsensitiveContains(query)
    }
    let pageSize = max(1, request.dataSource.paging?.size ?? max(1, schedules.count))
    let page = max(1, request.input.page ?? 1)
    let start = min(schedules.count, (page - 1) * pageSize)
    let rows = try schedules.dropFirst(start).prefix(pageSize).map(scheduleForgeRow)
    let pageCount = schedules.isEmpty ? 0 : (schedules.count + pageSize - 1) / pageSize
    return ForgeRuntime.DataSourceFetchResult(
        rows: rows,
        metrics: [
            "pageCount": .number(Double(pageCount)),
            "totalCount": .number(Double(schedules.count))
        ]
    )
}

private func scheduleForgeRow(_ schedule: Schedule) throws -> [String: ForgeIOSRuntime.JSONValue] {
    let data = try JSONEncoder.agently().encode(schedule)
    let value = try JSONDecoder.agently().decode(AgentlySDK.JSONValue.self, from: data)
    return value.forgeValue.objectValue ?? [:]
}

private func loadWorkspaceOptions(
    client: AgentlyClient,
    request: ForgeRuntime.DataSourceFetchRequest
) async throws -> ForgeRuntime.DataSourceFetchResult {
    let metadata = try await client.getWorkspaceMetadata()
    let rows: [[String: ForgeIOSRuntime.JSONValue]]
    switch request.dataSource.selectors?.data?.trimmingCharacters(in: .whitespacesAndNewlines) {
    case "agentInfos":
        rows = metadata.agentInfos.map {
            [
                "id": .string($0.agentID ?? $0.id),
                "name": .string($0.name ?? $0.agentID ?? "Agent"),
                "modelRef": .string($0.modelRef ?? "")
            ]
        }
    case "modelInfos":
        let models = metadata.modelInfos.map {
            [
                "id": ForgeIOSRuntime.JSONValue.string($0.modelID ?? $0.id),
                "name": .string($0.name ?? $0.modelID ?? "Model")
            ]
        }
        rows = models.isEmpty
            ? metadata.models.map { ["id": .string($0), "name": .string($0)] }
            : models
    default:
        rows = []
    }
    return ForgeRuntime.DataSourceFetchResult(rows: rows)
}

private func loadPublicAgents(client: AgentlyClient) async throws -> ForgeRuntime.DataSourceFetchResult {
    let agents = try await client.getPublicAgents()
    return ForgeRuntime.DataSourceFetchResult(rows: agents.map {
        [
            "id": .string($0.agentID ?? $0.id),
            "name": .string($0.name ?? $0.agentID ?? "Agent"),
            "modelRef": .string($0.modelRef ?? "")
        ]
    })
}

private func loadScheduleRuns(
    client: AgentlyClient,
    request: ForgeRuntime.DataSourceFetchRequest
) async throws -> ForgeRuntime.DataSourceFetchResult {
    let filters = request.input.filter.reduce(into: [String: String]()) { result, entry in
        if let value = entry.value.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines), !value.isEmpty {
            result[entry.key] = value
        }
    }
    let scheduleID = request.resolvedInputs["scheduleId"]?.stringValue
        ?? request.input.parameters["scheduleId"]?.stringValue
    let page = max(1, request.input.page ?? 1)
    let size = max(1, request.dataSource.paging?.size ?? 10)
    let response = try await client.listScheduleRuns(
        scheduleID: scheduleID,
        filters: filters,
        page: page,
        size: size
    )
    let rows = try response.rows.map { run -> [String: ForgeIOSRuntime.JSONValue] in
        let data = try JSONEncoder.agently().encode(run)
        let value = try JSONDecoder.agently().decode(AgentlySDK.JSONValue.self, from: data)
        return value.forgeValue.objectValue ?? [:]
    }
    return ForgeRuntime.DataSourceFetchResult(
        rows: rows,
        metrics: [
            "pageCount": .number(Double(response.pageCount)),
            "totalCount": .number(Double(response.totalCount))
        ]
    )
}

internal func extractDatasourceID(from uri: String) -> String? {
    let trimmed = uri.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !trimmed.isEmpty else { return nil }
    let marker = "/datasources/"
    guard let markerRange = trimmed.range(of: marker) else { return nil }
    var suffix = String(trimmed[markerRange.upperBound...])
    if let queryRange = suffix.range(of: "?") {
        suffix = String(suffix[..<queryRange.lowerBound])
    }
    if suffix.hasSuffix("/fetch") {
        suffix.removeLast("/fetch".count)
    }
    let datasourceID = suffix.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
    guard !datasourceID.isEmpty else { return nil }
    return datasourceID.removingPercentEncoding ?? datasourceID
}
