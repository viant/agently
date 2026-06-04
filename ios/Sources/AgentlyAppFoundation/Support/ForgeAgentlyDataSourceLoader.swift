import AgentlySDK
import ForgeIOSRuntime

func makeForgeAgentlyDataSourceLoader(
    client: AgentlyClient
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

        let response = try await client.fetchDatasource(
            FetchDatasourceInput(
                id: datasourceID,
                inputs: inputs.isEmpty ? nil : inputs.mapValues(\.appValue)
            )
        )

        return ForgeRuntime.DataSourceFetchResult(
            rows: response.rows.map { $0.mapValues(\.forgeValue) },
            metrics: response.metrics?.mapValues(\.forgeValue) ?? [:]
        )
    }
}
