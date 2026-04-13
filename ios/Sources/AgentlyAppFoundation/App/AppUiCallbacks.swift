import Foundation

public struct AppUiCallbacks {
    public let onSend: @MainActor () -> Void

    public init(onSend: @escaping @MainActor () -> Void) {
        self.onSend = onSend
    }
}
