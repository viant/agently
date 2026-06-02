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

        var inputs = request.input.parameters
        if !request.input.filter.isEmpty {
            var inputObject = inputs["input"]?.objectValue ?? [:]
            var queryObject = inputObject["query"]?.objectValue ?? [:]
            queryObject.merge(request.input.filter) { _, new in new }
            inputObject["query"] = .object(queryObject)
            inputs["input"] = .object(inputObject)
        }
        if let page = request.input.page {
            inputs["page"] = .number(Double(page))
        }

        let response = try await client.fetchDatasource(
            FetchDatasourceInput(
                id: datasourceID,
                inputs: inputs.isEmpty ? nil : inputs.mapValues(\.appValue)
            )
        )

        return ForgeRuntime.DataSourceFetchResult(
            rows: response.rows.map { $0.mapValues(\.forgeValue) },
            metrics: response.dataInfo?.mapValues(\.forgeValue) ?? [:]
        )
    }
}
