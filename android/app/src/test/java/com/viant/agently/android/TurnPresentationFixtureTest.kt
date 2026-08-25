package com.viant.agently.android

import com.viant.agentlysdk.ConversationState
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.PlannerState
import com.viant.agentlysdk.TurnState
import com.viant.agentlysdk.stream.BufferedMessage
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import com.viant.agentlysdk.stream.LiveExecutionGroup
import com.viant.agentlysdk.stream.LiveToolStepState
import com.viant.agentlysdk.stream.PlannedToolCall
import java.io.File
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class TurnPresentationFixtureTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test
    fun sharedProgressFixturesDriveAndroidAdapter() {
        val fixture = loadFixture()
        fixture.progressCases.forEach { testCase ->
            val input = testCase.input
            val terminal = testCase.expected == null
            val groups = input.groups.mapIndexed { index, group ->
                LiveExecutionGroup(
                    pageId = "fixture-page-$index",
                    assistantMessageId = "fixture-message-$index",
                    turnId = input.turnId,
                    status = input.status,
                    toolSteps = group.toolSteps.map {
                        LiveToolStepState(toolCallId = it.toolCallId, toolName = it.toolName, status = it.status)
                    },
                    toolCallsPlanned = group.toolCallsPlanned.map {
                        PlannedToolCall(toolCallId = it.toolCallId, toolName = it.toolName)
                    }
                )
            }
            val snapshot = if (terminal) null else ConversationStreamSnapshot(
                conversationId = "fixture",
                activeTurnId = input.turnId,
                feeds = emptyList(),
                pendingElicitation = null,
                bufferedMessages = emptyList(),
                liveExecutionGroupsById = groups.associateBy { it.assistantMessageId },
                plannerByTurnId = if (input.phase?.contains("plan", ignoreCase = true) == true) {
                    mapOf(input.turnId to PlannerState(status = "running"))
                } else emptyMap()
            )
            val state = if (terminal) null else ConversationStateResponse(
                conversation = ConversationState(
                    conversationId = "fixture",
                    turns = listOf(TurnState(turnId = input.turnId, status = input.status))
                )
            )
            val actual = turnProgressPresentation(!terminal, state, snapshot)
            val expected = testCase.expected
            if (expected == null) {
                assertNull(testCase.name, actual)
            } else {
                assertEquals(testCase.name, expectedActivity(expected), actual?.activity)
                assertEquals(testCase.name, expectedToolProgress(expected), actual?.toolProgress)
                expected.canStop?.let { assertEquals(testCase.name, it, actual?.canStop) }
            }
        }
    }

    @Test
    fun sharedNarrationFixturesDriveAndroidAdapter() {
        val fixture = loadFixture()
        fixture.narrationCases.forEach { testCase ->
            val input = testCase.input
            val messageId = input.narrationMessageId ?: "${input.turnId}:narration"
            val snapshot = ConversationStreamSnapshot(
                conversationId = "fixture",
                activeTurnId = input.turnId,
                feeds = emptyList(),
                pendingElicitation = null,
                bufferedMessages = listOf(
                    BufferedMessage(
                        id = messageId,
                        turnId = input.turnId,
                        role = "assistant",
                        narration = input.candidates.firstOrNull().orEmpty(),
                        status = "running"
                    )
                ),
                liveExecutionGroupsById = emptyMap()
            )
            val actual = transcriptWithActiveAssistant(emptyList(), snapshot).lastOrNull()
            assertEquals(testCase.name, testCase.expected?.content, actual?.markdown)
            if (testCase.expected == null) assertNull(testCase.name, actual)
            else assertEquals(testCase.name, testCase.expected.messageId, actual?.id)
        }
    }

    private fun loadFixture(): TurnPresentationFixture {
        val candidates = listOf(
            File("../../agently-core/sdk/fixtures/turn_presentation.json"),
            File("../../../agently-core/sdk/fixtures/turn_presentation.json"),
            File("deps/agently-core/sdk/fixtures/turn_presentation.json"),
            File("../deps/agently-core/sdk/fixtures/turn_presentation.json")
        )
        val fixture = candidates.firstOrNull(File::isFile)
            ?: error("shared turn presentation fixture not found: ${candidates.joinToString()}")
        val root = json.parseToJsonElement(fixture.readText()).jsonObject
        return TurnPresentationFixture(
            progressCases = root.getValue("progressCases").jsonArray.map { element ->
                val row = element.jsonObject
                val input = row.getValue("input").jsonObject
                val expectedElement = row["expected"]
                FixtureProgressCase(
                    name = row.string("name"),
                    input = FixtureProgressInput(
                        turnId = input.string("turnId"),
                        status = input.string("status"),
                        phase = input.optionalString("phase"),
                        groups = input["groups"]?.jsonArray?.map { groupElement ->
                            val group = groupElement.jsonObject
                            FixtureGroup(
                                toolSteps = group.parseTools("toolSteps"),
                                toolCallsPlanned = group.parseTools("toolCallsPlanned")
                            )
                        }.orEmpty()
                    ),
                    expected = if (expectedElement == null || expectedElement is JsonNull) null else {
                        val expected = expectedElement.jsonObject
                        val activity = expected["activity"]?.jsonObject
                        FixtureProgressExpected(
                            activity = activity?.let { FixtureActivity(it.string("kind"), it.optionalString("label")) },
                            completedToolCount = expected.int("completedToolCount"),
                            activeToolCount = expected.int("activeToolCount"),
                            queuedToolCount = expected.int("queuedToolCount"),
                            failedToolCount = expected.int("failedToolCount"),
                            totalToolCount = expected.int("totalToolCount"),
                            identityComplete = expected.bool("identityComplete"),
                            canStop = expected.bool("canStop")
                        )
                    }
                )
            },
            narrationCases = root.getValue("narrationCases").jsonArray.map { element ->
                val row = element.jsonObject
                val input = row.getValue("input").jsonObject
                val expectedElement = row["expected"]
                FixtureNarrationCase(
                    name = row.string("name"),
                    input = FixtureNarrationInput(
                        turnId = input.string("turnId"),
                        narrationMessageId = input.optionalString("narrationMessageId"),
                        candidates = input["candidates"]?.jsonArray?.mapNotNull { it.jsonPrimitive.contentOrNull }.orEmpty()
                    ),
                    expected = if (expectedElement == null || expectedElement is JsonNull) null else {
                        val expected = expectedElement.jsonObject
                        FixtureNarrationExpected(expected.string("messageId"), expected.string("content"))
                    }
                )
            }
        )
    }

    private fun expectedActivity(expected: FixtureProgressExpected): String = when (expected.activity?.kind) {
        "connecting" -> "Connecting"
        "planning" -> "Planning"
        "stopping" -> "Stopping"
        "tool" -> expected.activity.label ?: "Using tool"
        "tools" -> "Calling tools"
        "waiting_for_user" -> "Needs your input"
        "writing" -> "Writing response"
        else -> "Thinking"
    }

    private fun expectedToolProgress(expected: FixtureProgressExpected): String? {
        if (expected.identityComplete == false || (expected.totalToolCount ?: 0) <= 0) return null
        return buildList {
            add("${expected.completedToolCount ?: 0}/${expected.totalToolCount ?: 0} done")
            if ((expected.activeToolCount ?: 0) > 0) add("${expected.activeToolCount} active")
            if ((expected.queuedToolCount ?: 0) > 0) add("${expected.queuedToolCount} queued")
            if ((expected.failedToolCount ?: 0) > 0) add("${expected.failedToolCount} failed")
        }.joinToString(" · ")
    }
}

private data class TurnPresentationFixture(
    val progressCases: List<FixtureProgressCase>,
    val narrationCases: List<FixtureNarrationCase>
)

private data class FixtureProgressCase(
    val name: String,
    val input: FixtureProgressInput,
    val expected: FixtureProgressExpected? = null
)

private data class FixtureProgressInput(
    val turnId: String,
    val status: String,
    val phase: String? = null,
    val groups: List<FixtureGroup> = emptyList()
)

private data class FixtureGroup(
    val toolSteps: List<FixtureTool> = emptyList(),
    val toolCallsPlanned: List<FixtureTool> = emptyList()
)

private data class FixtureTool(
    val toolCallId: String? = null,
    val toolName: String? = null,
    val status: String? = null
)

private data class FixtureProgressExpected(
    val activity: FixtureActivity? = null,
    val completedToolCount: Int? = null,
    val activeToolCount: Int? = null,
    val queuedToolCount: Int? = null,
    val failedToolCount: Int? = null,
    val totalToolCount: Int? = null,
    val identityComplete: Boolean? = null,
    val canStop: Boolean? = null
)

private data class FixtureActivity(val kind: String, val label: String? = null)

private data class FixtureNarrationCase(
    val name: String,
    val input: FixtureNarrationInput,
    val expected: FixtureNarrationExpected? = null
)

private data class FixtureNarrationInput(
    val turnId: String,
    val narrationMessageId: String? = null,
    val candidates: List<String> = emptyList()
)

private data class FixtureNarrationExpected(val messageId: String, val content: String)

private fun JsonObject.string(key: String): String = getValue(key).jsonPrimitive.content
private fun JsonObject.optionalString(key: String): String? = get(key)?.jsonPrimitive?.contentOrNull
private fun JsonObject.int(key: String): Int? = get(key)?.jsonPrimitive?.intOrNull
private fun JsonObject.bool(key: String): Boolean? = get(key)?.jsonPrimitive?.booleanOrNull
private fun JsonObject.parseTools(key: String): List<FixtureTool> = get(key)?.jsonArray?.map { element ->
    val tool = element.jsonObject
    FixtureTool(tool.optionalString("toolCallId"), tool.optionalString("toolName"), tool.optionalString("status"))
}.orEmpty()
