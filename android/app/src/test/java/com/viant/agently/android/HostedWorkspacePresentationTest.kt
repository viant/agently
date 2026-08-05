package com.viant.agently.android

import com.viant.agentlysdk.WorkspaceWindowSnapshot
import org.junit.Assert.assertEquals
import org.junit.Test

class HostedWorkspacePresentationTest {
    @Test
    fun `resolve hosted workspace presentation uses title when available`() {
        val window = WorkspaceWindowSnapshot(
            windowId = "line_1",
            windowKey = "line",
            windowTitle = "OLV_BAU_AUS_Media.net PMP"
        )

        val presentation = resolveHostedWorkspacePresentation(window)

        assertEquals("Line", presentation?.badgeLabel)
        assertEquals("OLV_BAU_AUS_Media.net PMP", presentation?.title)
        assertEquals(null, presentation?.subtitle)
    }

    @Test
    fun `resolve hosted workspace presentation falls back to humanized window key`() {
        val window = WorkspaceWindowSnapshot(
            windowId = "order_1",
            windowKey = "order",
            windowTitle = "order"
        )

        val presentation = resolveHostedWorkspacePresentation(window)

        assertEquals("Order", presentation?.badgeLabel)
        assertEquals("Order", presentation?.title)
        assertEquals(null, presentation?.subtitle)
    }

    @Test
    fun `resolve hosted workspace presentation splits camel case window keys`() {
        val reportBuilder = resolveHostedWorkspacePresentation(
            WorkspaceWindowSnapshot(
                windowId = "reportBuilder_1",
                windowKey = "reportBuilder",
                windowTitle = "reportBuilder"
            )
        )
        val forecasting = resolveHostedWorkspacePresentation(
            WorkspaceWindowSnapshot(
                windowId = "forecastingCubeBuilder_1",
                windowKey = "forecastingCubeBuilder",
                windowTitle = "forecastingCubeBuilder"
            )
        )

        assertEquals("Report Builder", reportBuilder?.badgeLabel)
        assertEquals("Report Builder", reportBuilder?.title)
        assertEquals("Forecasting Cube Builder", forecasting?.badgeLabel)
        assertEquals("Forecasting Cube Builder", forecasting?.title)
    }
}
