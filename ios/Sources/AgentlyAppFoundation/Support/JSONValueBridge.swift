import AgentlySDK
import ForgeIOSRuntime

public typealias AppJSONValue = AgentlySDK.JSONValue
public typealias ForgeJSONValue = ForgeIOSRuntime.JSONValue

extension AgentlySDK.JSONValue {
    var forgeValue: ForgeIOSRuntime.JSONValue {
        switch self {
        case .string(let value):
            return .string(value)
        case .number(let value):
            return .number(value)
        case .bool(let value):
            return .bool(value)
        case .null:
            return .null
        case .array(let values):
            return .array(values.map(\.forgeValue))
        case .object(let values):
            return .object(values.mapValues { $0.forgeValue })
        }
    }
}

extension ForgeIOSRuntime.JSONValue {
    var appValue: AgentlySDK.JSONValue {
        switch self {
        case .string(let value):
            return .string(value)
        case .number(let value):
            return .number(value)
        case .bool(let value):
            return .bool(value)
        case .null:
            return .null
        case .array(let values):
            return .array(values.map(\.appValue))
        case .object(let values):
            return .object(values.mapValues { $0.appValue })
        }
    }
}
