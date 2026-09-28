package dev.tavernagent.mobile.domain

import dev.tavernagent.mobile.data.remote.SseFrame
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TurnEventCodecTest {

    @Test
    fun `parses block delta`() {
        val event = TurnEventCodec.parse(
            SseFrame(
                event = "block.delta",
                data = """{"seq":2,"kind":"dialogue","speakerId":"npc_1","delta":"「来了」"}""",
                id = null,
            ),
        )
        assertEquals(TurnEvent.BlockDelta(2, "dialogue", "npc_1", "「来了」"), event)
    }

    @Test
    fun `parses thinking delta`() {
        val event = TurnEventCodec.parse(
            SseFrame("turn.thinking", """{"thinking":"权衡","attemptId":"a1"}""", null),
        )
        assertEquals(TurnEvent.ThinkingDelta("权衡"), event)
    }

    @Test
    fun `parses terminal events`() {
        val committed = TurnEventCodec.parse(SseFrame("turn.committed", """{"nodeId":"n9"}""", "turn:12"))
        assertEquals(TurnEvent.Committed("n9"), committed)
        assertTrue(TurnEventCodec.isTerminal(SseFrame("turn.committed", "{}", null)))
        assertTrue(TurnEventCodec.isTerminal(SseFrame("turn.failed", "{}", null)))
    }

    @Test
    fun `unknown event is ignored`() {
        assertEquals(null, TurnEventCodec.parse(SseFrame("session.updated", "{}", null)))
    }

    @Test
    fun `status mapping covers continuation and approval`() {
        assertEquals(TurnEvent.Truncated, TurnEventCodec.fromStatus("awaiting_continuation"))
        assertEquals(TurnEvent.AwaitingConfirm, TurnEventCodec.fromStatus("awaiting_approval"))
        assertEquals(TurnEvent.Committed(null), TurnEventCodec.fromStatus("committed"))
        assertTrue(TurnEventCodec.fromStatus("generating") is TurnEvent.Snapshot)
    }

    @Test
    fun `status mapping keeps server failure code and message`() {
        assertEquals(
            TurnEvent.Failed("PROVIDER_UNAVAILABLE", "上游不可用"),
            TurnEventCodec.fromStatus("failed", failureCode = "PROVIDER_UNAVAILABLE", failureMessage = "上游不可用"),
        )
        // 缺省时仍是可读的兜底文案。
        assertEquals(TurnEvent.Failed("", "生成失败"), TurnEventCodec.fromStatus("failed"))
    }
}
