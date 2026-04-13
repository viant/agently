import Foundation

public enum AppEffects {
    public static func bootstrap(_ runtime: AppRuntime) {
        Task { await runtime.bootstrap() }
    }
}
