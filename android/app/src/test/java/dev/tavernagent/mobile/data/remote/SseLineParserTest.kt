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
}
