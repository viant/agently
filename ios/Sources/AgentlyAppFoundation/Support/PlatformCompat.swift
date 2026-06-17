import Foundation
import SwiftUI

#if canImport(UIKit)
import UIKit
#elseif canImport(AppKit)
import AppKit
#endif

@MainActor
func dismissAgentlyPlatformKeyboard() {
    #if canImport(UIKit)
    UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
    #elseif canImport(AppKit)
    NSApp.keyWindow?.makeFirstResponder(nil)
    #endif
}

@MainActor
func requestAgentlyPlatformKeyboardDismissal() {
    dismissAgentlyPlatformKeyboard()
    NotificationCenter.default.post(name: .agentlyKeyboardDismissalRequested, object: nil)
}

extension Notification.Name {
    static let agentlyKeyboardDismissalRequested = Notification.Name("com.viant.agently.keyboardDismissalRequested")
}

extension Color {
    static var agentlySystemBackground: Color {
        #if canImport(UIKit)
        return Color(uiColor: .systemBackground)
        #elseif canImport(AppKit)
        return Color(nsColor: .windowBackgroundColor)
        #else
        return .white
        #endif
    }

    static var agentlySecondarySystemBackground: Color {
        #if canImport(UIKit)
        return Color(uiColor: .secondarySystemBackground)
        #elseif canImport(AppKit)
        return Color(nsColor: .controlBackgroundColor)
        #else
        return Color.black.opacity(0.04)
        #endif
    }
}

extension View {
    @ViewBuilder
    func agentlyLookupPresentation<Item: Identifiable, Content: View>(
        item: Binding<Item?>,
        @ViewBuilder content: @escaping (Item) -> Content
    ) -> some View {
        #if os(iOS)
        self.fullScreenCover(item: item, content: content)
        #else
        self.sheet(item: item, content: content)
        #endif
    }

    @ViewBuilder
    func agentlyInlineTitleMode() -> some View {
        #if os(iOS)
        self.navigationBarTitleDisplayMode(.inline)
        #else
        self
        #endif
    }

    @ViewBuilder
    func agentlyLookupSearchable(text: Binding<String>) -> some View {
        #if os(iOS)
        self.searchable(text: text, placement: .navigationBarDrawer(displayMode: .always))
        #else
        self.searchable(text: text)
        #endif
    }

    @ViewBuilder
    func agentlyScrollDismissesKeyboard() -> some View {
        #if os(iOS)
        self.scrollDismissesKeyboard(.interactively)
        #else
        self
        #endif
    }
}
