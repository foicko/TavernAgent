package dev.tavernagent.mobile.data.remote

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class SseLineParserTest {

    @Test
    fun `dispatches frame with event data and id`() {
        val parser = SseLineParser()
        assertNull(parser.accept("event: block.delta"))
        assertNull(parser.accept("id: agg:7"))
        assertNull(parser.accept("data: {\"seq\":1}"))
        val frame = parser.accept("")
        assertEquals("block.delta", frame?.event)
        assertEquals("agg:7", frame?.id)
        assertEquals("{\"seq\":1}", frame?.data)
    }

    @Test
    fun `ignores heartbeat comments`() {
        val parser = SseLineParser()
        assertNull(parser.accept(": ping"))
        assertNull(parser.accept(""))
    }

    @Test
    fun `joins multi-line data with newline`() {
        val parser = SseLineParser()
        parser.accept("data: line1")
        parser.accept("data: line2")
        val frame = parser.accept("")
        assertEquals("line1\nline2", frame?.data)
    }

    @Test
    fun `strips a single leading space in value`() {
        val parser = SseLineParser()
        parser.accept("data:  two-spaces")
        assertEquals(" two-spaces", parser.accept("")?.data)
    }

    @Test
    fun `blank line without data does not dispatch`() {
        val parser = SseLineParser()
        assertNull(parser.accept("event: turn.thinking"))
        assertNull(parser.accept(""))
    }

    @Test
    fun `end of stream flushes pending frame`() {
        val parser = SseLineParser()
        parser.accept("event: turn.committed")
        parser.accept("data: {\"nodeId\":\"n1\"}")
        val frame = parser.endOfStream()
        assertEquals("turn.committed", frame?.event)
    }

    @Test
    fun `empty event name falls back to message`() {
        val parser = SseLineParser()
        assertNull(parser.accept("event:"))
        assertNull(parser.accept("data: x"))
        // 规范：事件名缓冲为空时按 message 派发，而不是产出 event="" 被消费端丢弃。
        assertEquals("message", parser.accept("")?.event)
    }

    @Test
    fun `retry is parsed and exposed as stream level hint`() {
        val parser = SseLineParser()
        assertNull(parser.accept("retry: 2500"))
        parser.accept("data: x")
        assertEquals(2500L, parser.accept("")?.retryMillis)

        // 非数字与负数按规范忽略，沿用上一个有效值。
        parser.accept("retry: nope")
        parser.accept("retry: -1")
        parser.accept("data: y")
        assertEquals(2500L, parser.accept("")?.retryMillis)
    }

    @Test
    fun `frame without retry carries null`() {
        val parser = SseLineParser()
        parser.accept("data: x")
        assertNull(parser.accept("")?.retryMillis)
    }
}
