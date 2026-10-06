import Foundation
import AgentlySDK
import ForgeIOSRuntime
internal struct NativeReportCommandIdentity {
    let requestID: String
    let reportAdmissionRef: String?
}
internal func nativeReportCommandIdentity(_ params: [String: AgentlySDK.JSONValue]) throws -> NativeReportCommandIdentity {
    let hasID = params.keys.contains("requestId"), hasRef = params.keys.contains("reportAdmissionRef")
    if !hasID && !hasRef { return .init(requestID: "native-" + UUID().uuidString, reportAdmissionRef: nil) }
    guard hasID && hasRef, case .string(let id)? = params["requestId"], UUID(uuidString: id) != nil,
          id.count == 36, case .string(let ref)? = params["reportAdmissionRef"], !ref.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
        throw NativeReportPersistenceError.invalidIdentity
    }
    return .init(requestID: id, reportAdmissionRef: ref)
}
internal func nativeReportRequestedParamsMatch(_ actual: AgentlySDK.JSONValue?, expected: [String: ForgeIOSRuntime.JSONValue], requestID: String, ref: String?) -> Bool {
    let key = "_agentlyForecastCommand"
    guard expected[key] == nil, var value = actual?.forgeValue.objectValue else { return false }
    if let ref {
        guard value.removeValue(forKey: key) == .object(["version": .number(1), "ref": .string(ref), "requestId": .string(requestID)]) else { return false }
    } else if value[key] != nil { return false }
    return value == expected
}
