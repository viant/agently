package com.viant.agently.android

import com.viant.agentlysdk.*
import org.junit.Assert.*
import org.junit.Test

class PersistedToolExecutionTest {
    @Test fun coldTerminalHistoryPreservesRepeatedToolNamesAndAllTurns() {
        fun turn(id: String, tools: List<ToolStepState>) = TurnState(id, status = "succeeded", execution = ExecutionState(listOf(ExecutionPageState("page-$id", toolSteps = tools))))
        val state = ConversationStateResponse(conversation = ConversationState("conversation", listOf(
            turn("first", listOf(ToolStepState("call-a", toolName = "ui.report.getCurrent", status = "succeeded"), ToolStepState("call-b", toolName = "ui.report.getCurrent", status = "succeeded"))),
            turn("second", listOf(ToolStepState("call-c", toolName = "ui.report.run", status = "succeeded"))), turn("empty", emptyList()))))
        val turns = persistedToolExecutionTurns(state)
        assertEquals(listOf("first", "second"), turns.map { it.turnId })
        assertEquals(listOf("call-a", "call-b", "call-c"), turns.flatMap { it.execution!!.pages }.flatMap { it.toolSteps }.map { it.toolCallId })
    }
}
