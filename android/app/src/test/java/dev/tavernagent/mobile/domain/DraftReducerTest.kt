package dev.tavernagent.mobile.domain

import dev.tavernagent.mobile.data.remote.dto.InFlightDto
import org.junit.Assert.assertEquals
import org.junit.Test

class DraftReducerTest {

    @Test
    fun `delta with seq creates and appends blocks`() {
        var blocks = DraftReducer.applyDelta(emptyList(), 1, "narration", null, "夜色")
        blocks = DraftReducer.applyDelta(blocks, 1, "narration", null, "渐深")
        blocks = DraftReducer.applyDelta(blocks, 3, "dialogue", "npc_1", "「谁？」")
        assertEquals(3, blocks.size)
        assertEquals("夜色渐深", blocks[0].text)
        assertEquals("", blocks[1].text)
        assertEquals("「谁？」", blocks[2].text)
        assertEquals("dialogue", blocks[2].kind)
    }

    @Test
    fun `seq zero appends to the last block`() {
        val blocks = DraftReducer.applyDelta(
            DraftReducer.applyDelta(emptyList(), 1, "narration", null, "A"),
            0,
            "narration",
            null,
            "B",
        )
        assertEquals(1, blocks.size)
        assertEquals("AB", blocks[0].text)
    }

    @Test
    fun `in-flight snapshot only lengthens text`() {
        val blocks = listOf(DraftBlock(seq = 1, text = "完整的句子"))
        val shorter = DraftReducer.applyInFlight(blocks, InFlightDto(seq = 1, text = "完整的"))
        assertEquals("完整的句子", shorter[0].text)
        val longer = DraftReducer.applyInFlight(blocks, InFlightDto(seq = 1, text = "完整的句子，还有后半段"))
        assertEquals("完整的句子，还有后半段", longer[0].text)
    }

    @Test
    fun `block appended replaces the frame`() {
        val blocks = DraftReducer.applyDelta(emptyList(), 1, "narration", null, "残")
        val replaced = DraftReducer.applyBlockAppended(blocks, 1, "narration", null, "完整段落")
        assertEquals("完整段落", replaced[0].text)
    }
}
