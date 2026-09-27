package dev.tavernagent.mobile.domain

import dev.tavernagent.mobile.data.remote.dto.PlotNodeDto
import dev.tavernagent.mobile.data.remote.dto.SessionViewDto
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class StoryMessagesTest {

    private fun rootNode() = PlotNodeDto(
        nodeId = "root",
        kind = "root",
        depth = 0,
        contentJson = """{"openingText":"雨落在窗上。"}""",
    )

    private fun turnNode(id: String, parent: String, depth: Int, turn: Int) = PlotNodeDto(
        nodeId = id,
        parentId = parent,
        kind = "turn",
        depth = depth,
        turnNumber = turn,
        contentJson = """
            {"inputKind":"text","inputText":"我推门进去","inputNote":"放慢",
             "blocks":[{"kind":"narration","text":"门轴轻响。","segments":[]},
                       {"kind":"dialogue","speakerId":"npc_1","text":"「来了？」"}],
             "options":[{"optionId":"o1","intent":"clever","text":"试探她"}],
             "uiHints":[{"kind":"scene","values":{"title":"酒馆"},"text":"酒馆"}]}
        """.trimIndent(),
    )

    private fun memoryNode() = PlotNodeDto(
        nodeId = "mem1",
        parentId = "n1",
        kind = "memory_change",
        depth = 2,
        contentJson = "{}",
    )

    @Test
    fun `derives root and turn messages in order`() {
        val view = SessionViewDto(
            sessionId = "s1",
            nodes = listOf(turnNode("n1", "root", 1, 1), memoryNode(), rootNode()),
        )
        val messages = StoryMessages.fromView(view)
        assertEquals(2, messages.size)
        assertTrue(messages[0].isRoot)
        assertEquals("雨落在窗上。", messages[0].blocks.first().text)
        assertEquals("我推门进去", messages[1].inputText)
        assertEquals("放慢", messages[1].inputNote)
        assertEquals(2, messages[1].blocks.size)
        assertEquals("o1", messages[1].options.first().optionId)
        assertEquals("酒馆", messages[1].uiHints.first().values["title"])
    }

    @Test
    fun `attaches thinking by node or turn id`() {
        val view = SessionViewDto(
            sessionId = "s1",
            nodes = listOf(
                rootNode(),
                turnNode("n1", "root", 1, 1).copy(
                    contentJson = """{"inputKind":"text","inputText":"x",
                        "provenance":{"turnId":"t1","attemptId":"a1","mode":"structured"},
                        "blocks":[]}""",
                ),
            ),
        )
        val messages = StoryMessages.fromView(view, thinking = mapOf("t1" to "权衡", "n2" to "其他"), seconds = mapOf("n1" to 7))
        assertEquals("权衡", messages[1].thinking)
        assertEquals(7, messages[1].thinkingSeconds)
    }
}
