package dev.tavernagent.mobile.util

import kotlinx.coroutines.CancellationException
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

class CoroutinesTest {

    @Test
    fun `wraps ordinary failures into Result`() {
        val result = runCatchingCancellable { error("boom") }
        assertTrue(result.isFailure)
        assertEquals("boom", result.exceptionOrNull()?.message)
    }

    @Test
    fun `returns success unchanged`() {
        assertEquals(42, runCatchingCancellable { 42 }.getOrNull())
    }

    @Test
    fun `propagates cancellation instead of swallowing it`() {
        try {
            runCatchingCancellable { throw CancellationException("cancelled") }
            fail("CancellationException 必须继续传播（结构化并发）")
        } catch (expected: CancellationException) {
            assertEquals("cancelled", expected.message)
        }
    }
}
