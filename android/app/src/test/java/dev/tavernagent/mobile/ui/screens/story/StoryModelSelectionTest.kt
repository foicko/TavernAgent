package dev.tavernagent.mobile.ui.screens.story

import dev.tavernagent.mobile.data.remote.dto.ModelInstanceDto
import dev.tavernagent.mobile.data.remote.dto.SlotBindingDto
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class StoryModelSelectionTest {

    @Test
    fun `effortLabel returns localized labels`() {
        assertEquals("默认", effortLabel(""))
        assertEquals("默认", effortLabel(null))
        assertEquals("低", effortLabel("low"))
        assertEquals("中", effortLabel("medium"))
        assertEquals("高", effortLabel("high"))
    }

    @Test
    fun `resolves active primary model and reasoning effort from state`() {
        val stateDefault = StoryUiState()
        assertNull(stateDefault.activePrimaryModel)
        assertEquals("默认模型", stateDefault.activePrimaryModelName)
        assertEquals("", stateDefault.reasoningEffort)

        val model1 = ModelInstanceDto(
            id = "m1",
            name = "DeepSeek V3",
            model = "deepseek-chat",
            reasoningEffort = "high",
        )
        val model2 = ModelInstanceDto(
            id = "m2",
            name = "Claude 3.5 Sonnet",
            model = "claude-3-5-sonnet",
            reasoningEffort = "low",
        )
        val slotPrimary = SlotBindingDto(
            slot = "primary",
            enabled = true,
            modelId = "m1",
        )

        val stateConfigured = StoryUiState(
            models = listOf(model1, model2),
            slots = listOf(slotPrimary),
        )

        assertEquals("m1", stateConfigured.activePrimaryModel?.id)
        assertEquals("DeepSeek V3", stateConfigured.activePrimaryModelName)
        assertEquals("high", stateConfigured.reasoningEffort)
        assertEquals("高", effortLabel(stateConfigured.reasoningEffort))

        // Switch to model2
        val stateSwitched = stateConfigured.copy(
            slots = listOf(slotPrimary.copy(modelId = "m2")),
        )
        assertEquals("m2", stateSwitched.activePrimaryModel?.id)
        assertEquals("Claude 3.5 Sonnet", stateSwitched.activePrimaryModelName)
        assertEquals("low", stateSwitched.reasoningEffort)
        assertEquals("低", effortLabel(stateSwitched.reasoningEffort))
    }
}
