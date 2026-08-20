import Foundation
import SwiftUI

internal struct ComposerEditorProjection: Equatable {
    let source: String
    let display: String
    private let sourceToDisplayOffsets: [Int]
    private let displayToSourceOffsets: [Int]

    init(source: String, occurrences: [ComposerLookupOccurrence]) {
        self.source = source
        let nsSource = source as NSString
        let sourceLength = nsSource.length
        // Swift String.Index values are tied to the String instance that
        // created them. SwiftUI can briefly deliver a new binding value with
        // lookup occurrences computed from the previous value. Re-derive the
        // ranges against this exact source before converting them to NSRange;
        // using a stale range traps inside String.UTF16View._offsetRange.
        let currentOccurrences = parseComposerLookupOccurrences(
            query: source,
            registry: occurrences.map(\.entry)
        )
        let removedRanges = currentOccurrences
            .map { occurrence in
                expandedHiddenLookupRange(
                    NSRange(occurrence.displayRange, in: source),
                    source: nsSource
                )
            }
            .sorted { lhs, rhs in lhs.location < rhs.location }

        var display = ""
        var sourceToDisplay = Array(repeating: 0, count: sourceLength + 1)
        var displayToSource = [0]
        var sourceOffset = 0
        var displayOffset = 0

        func appendVisibleRange(_ range: NSRange) {
            guard range.length > 0 else { return }
            display += nsSource.substring(with: range)
            for delta in 0...range.length {
                sourceToDisplay[range.location + delta] = displayOffset + delta
            }
            for delta in 1...range.length {
                displayToSource.append(range.location + delta)
            }
            displayOffset += range.length
        }

        for range in removedRanges where range.location >= sourceOffset {
            appendVisibleRange(NSRange(location: sourceOffset, length: range.location - sourceOffset))
            let rangeEnd = min(sourceLength, NSMaxRange(range))
            if range.location <= rangeEnd {
                for offset in range.location...rangeEnd {
                    sourceToDisplay[offset] = displayOffset
                }
                displayToSource[displayOffset] = rangeEnd
            }
            sourceOffset = rangeEnd
        }
        appendVisibleRange(NSRange(location: sourceOffset, length: sourceLength - sourceOffset))

        self.display = display
        self.sourceToDisplayOffsets = sourceToDisplay
        self.displayToSourceOffsets = displayToSource
    }

    func displayOffset(forSourceOffset offset: Int) -> Int {
        sourceToDisplayOffsets[min(max(0, offset), sourceToDisplayOffsets.count - 1)]
    }

    func sourceOffset(forDisplayOffset offset: Int) -> Int {
        displayToSourceOffsets[min(max(0, offset), displayToSourceOffsets.count - 1)]
    }
}

private func expandedHiddenLookupRange(_ range: NSRange, source: NSString) -> NSRange {
    guard range.location != NSNotFound, range.length > 0 else { return range }
    var result = range
    let end = NSMaxRange(range)
    if end < source.length,
       range.location > 0,
       source.substring(with: NSRange(location: range.location - 1, length: 1)).allSatisfy(\.isWhitespace),
       source.substring(with: NSRange(location: end, length: 1)).allSatisfy(\.isWhitespace) {
        result.length += 1
    } else if range.location == 0,
              end < source.length,
              source.substring(with: NSRange(location: end, length: 1)).allSatisfy(\.isWhitespace) {
        result.length += 1
    }
    return result
}

#if os(iOS)
import UIKit

internal struct ComposerQueryEditor: UIViewRepresentable {
    @Binding var text: String
    @Binding var selectionUTF16Offset: Int
    let occurrences: [ComposerLookupOccurrence]
    let isDisabled: Bool
    var isFocused: FocusState<Bool>.Binding

    func makeCoordinator() -> Coordinator {
        Coordinator(parent: self)
    }

    func makeUIView(context: Context) -> UITextView {
        let textView = UITextView()
        textView.delegate = context.coordinator
        textView.font = .preferredFont(forTextStyle: .body)
        textView.adjustsFontForContentSizeCategory = true
        textView.backgroundColor = .clear
        textView.textContainerInset = .zero
        textView.textContainer.lineFragmentPadding = 0
        textView.autocorrectionType = .no
        textView.autocapitalizationType = .none
        textView.smartQuotesType = .no
        textView.smartDashesType = .no
        textView.keyboardDismissMode = .interactive
        textView.accessibilityIdentifier = "agently-composer-editor"
        return textView
    }

    func updateUIView(_ textView: UITextView, context: Context) {
        context.coordinator.parent = self
        let projection = ComposerEditorProjection(source: text, occurrences: occurrences)
        context.coordinator.projection = projection
        context.coordinator.isApplyingUpdate = true
        if textView.text != projection.display {
            textView.text = projection.display
        }
        textView.isEditable = !isDisabled
        let displaySelection = projection.displayOffset(forSourceOffset: selectionUTF16Offset)
        if textView.selectedRange.location != displaySelection || textView.selectedRange.length != 0 {
            textView.selectedRange = NSRange(location: displaySelection, length: 0)
        }
        context.coordinator.isApplyingUpdate = false

        if isFocused.wrappedValue, !textView.isFirstResponder {
            DispatchQueue.main.async { textView.becomeFirstResponder() }
        } else if !isFocused.wrappedValue, textView.isFirstResponder {
            DispatchQueue.main.async { textView.resignFirstResponder() }
        }
    }

    final class Coordinator: NSObject, UITextViewDelegate {
        var parent: ComposerQueryEditor
        var projection: ComposerEditorProjection
        var isApplyingUpdate = false

        init(parent: ComposerQueryEditor) {
            self.parent = parent
            self.projection = ComposerEditorProjection(source: parent.text, occurrences: parent.occurrences)
        }

        func textView(
            _ textView: UITextView,
            shouldChangeTextIn range: NSRange,
            replacementText replacement: String
        ) -> Bool {
            // Let UITextView own ordinary typing. Updating the SwiftUI binding
            // from shouldChangeText and returning false forced UIKit to rebuild
            // the text storage on every character, which made the keyboard-safe
            // composer alternate vertically between layout passes.
            if parent.occurrences.isEmpty {
                return true
            }
            let sourceStart = projection.sourceOffset(forDisplayOffset: range.location)
            let sourceEnd = projection.sourceOffset(forDisplayOffset: NSMaxRange(range))
            let sourceRange = NSRange(location: sourceStart, length: max(0, sourceEnd - sourceStart))
            parent.text = (projection.source as NSString).replacingCharacters(in: sourceRange, with: replacement)
            parent.selectionUTF16Offset = sourceStart + (replacement as NSString).length
            return false
        }

        func textViewDidChange(_ textView: UITextView) {
            guard !isApplyingUpdate, parent.occurrences.isEmpty else { return }
            parent.text = textView.text
            parent.selectionUTF16Offset = textView.selectedRange.location
            projection = ComposerEditorProjection(source: textView.text, occurrences: [])
        }

        func textViewDidChangeSelection(_ textView: UITextView) {
            guard !isApplyingUpdate else { return }
            parent.selectionUTF16Offset = projection.sourceOffset(
                forDisplayOffset: textView.selectedRange.location
            )
        }

        func textViewDidBeginEditing(_ textView: UITextView) {
            parent.isFocused.wrappedValue = true
        }

        func textViewDidEndEditing(_ textView: UITextView) {
            parent.isFocused.wrappedValue = false
        }
    }
}
#endif
