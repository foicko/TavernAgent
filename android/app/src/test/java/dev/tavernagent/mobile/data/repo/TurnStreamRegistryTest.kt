package dev.tavernagent.mobile.data.repo

import dev.tavernagent.mobile.data.local.ServerCredentials
import dev.tavernagent.mobile.data.remote.ApiClient
import dev.tavernagent.mobile.data.remote.ApiClientSource
import dev.tavernagent.mobile.domain.TurnPhase
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** 流式订阅与轮询对账的集成测试（SSE 增量 + 终态 + 轮询兜底）。 */
class TurnStreamRegistryTest {

    private lateinit var server: MockWebServer
    private lateinit var scope: CoroutineScope

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        scope = CoroutineScope(Dispatchers.Default)
    }

    @After
    fun tearDown() {
        // 断线不再自终止，必须显式取消作用域，否则重连协程会泄漏到后续用例/进程。
        scope.cancel()
        server.shutdown()
    }

    private fun sseWithTerminal() = buildString {
        append("event: turn.started\ndata: {\"turnId\":\"t1\",\"branchId\":\"b1\"}\n\n")
        append("event: turn.thinking\ndata: {\"thinking\":\"先稳住\"}\n\n")
        append("event: block.delta\ndata: {\"seq\":1,\"kind\":\"narration\",\"delta\":\"夜色\"}\n\n")
        append("event: block.delta\ndata: {\"seq\":1,\"kind\":\"narration\",\"delta\":\"渐深\"}\n\n")
        append("event: block.delta\ndata: {\"seq\":2,\"kind\":\"dialogue\",\"speakerId\":\"npc_1\",\"delta\":\"「谁？」\"}\n\n")
        append("event: turn.committed\ndata: {\"nodeId\":\"n2\"}\n\n")
    }

    private fun sseWithoutTerminal() = buildString {
        append("event: turn.started\ndata: {\"turnId\":\"t1\",\"branchId\":\"b1\"}\n\n")
        append("event: block.delta\ndata: {\"seq\":1,\"kind\":\"narration\",\"delta\":\"AB\"}\n\n")
    }

    /**
     * SSE 有终态：增量合并后由 turn.committed 结束。
     * 轮询端点返回 generating（不含 draft），避免与增量竞态。
     */
    private fun sseDispatcher(): Dispatcher = object : Dispatcher() {
        override fun dispatch(request: RecordedRequest): MockResponse {
            val path = request.path.orEmpty()
            return when {
                path.contains("/events") -> MockResponse()
                    .setHeader("Content-Type", "text/event-stream")
                    .setBody(sseWithTerminal())
                path.contains("/api/v1/turns/") -> MockResponse()
                    .setHeader("Content-Type", "application/json")
                    .setBody("""{"turnId":"t1","status":"generating","capabilities":{"canCancel":true,"terminal":false}}""")
                else -> MockResponse().setResponseCode(404)
            }
        }
    }

    /** SSE 断流且无终态：由轮询发现 committed（服务端权威状态兜底）。 */
    private fun pollingDispatcher(): Dispatcher {
        var polls = 0
        var streams = 0
        return object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                val path = request.path.orEmpty()
                return when {
                    path.contains("/events") -> {
                        streams++
                        // 真实的行内增量是临时事件、断线不重放：只有第一次连接带 delta。
                        val body = if (streams == 1) sseWithoutTerminal() else
                            "event: turn.started\ndata: {\"turnId\":\"t1\",\"branchId\":\"b1\"}\n\n"
                        MockResponse()
                            .setHeader("Content-Type", "text/event-stream")
                            .setBody(body)
                    }
                    path.contains("/api/v1/turns/") -> {
                        polls++
                        if (polls >= 3) {
                            MockResponse()
                                .setHeader("Content-Type", "application/json")
                                .setBody("""{"turnId":"t1","status":"committed","capabilities":{"terminal":true}}""")
                        } else {
                            MockResponse()
                                .setHeader("Content-Type", "application/json")
                                .setBody("""{"turnId":"t1","status":"generating","capabilities":{"terminal":false}}""")
                        }
                    }
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
    }

    private fun registry(): TurnStreamRegistry {
        val client = ApiClient(ServerCredentials(baseUrl = server.url("/").toString().trimEnd('/')))
        return TurnStreamRegistry(
            apiSource = ApiClientSource { client },
            scope = scope,
            reconcileIntervalMs = 100,
            backoffMs = longArrayOf(50, 100, 150, 200, 250),
        )
    }

    /** SSE 连续 N 次 5xx 后成功：断线属于可恢复，不应判失败。 */
    private fun flakySseDispatcher(failures: Int): Dispatcher {
        var streams = 0
        return object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse {
                val path = request.path.orEmpty()
                return when {
                    path.contains("/events") -> {
                        streams++
                        if (streams <= failures) {
                            MockResponse().setResponseCode(500).setBody("upstream boom")
                        } else {
                            MockResponse()
                                .setHeader("Content-Type", "text/event-stream")
                                .setBody(sseWithTerminal())
                        }
                    }
                    path.contains("/api/v1/turns/") -> MockResponse()
                        .setHeader("Content-Type", "application/json")
                        .setBody("""{"turnId":"t1","status":"generating","capabilities":{"terminal":false}}""")
                    else -> MockResponse().setResponseCode(404)
                }
            }
        }
    }

    @Test
    fun `collects draft blocks thinking and reaches committed`() = runBlocking {
        server.dispatcher = sseDispatcher()
        val registry = registry()
        registry.ensureStreaming("s1", "b1", "t1", "c1", "测试故事")

        val state = withTimeout(10_000) {
            registry.state("t1").first { it.phase == TurnPhase.COMMITTED }
        }
        assertEquals("先稳住", state.thinking)
        assertEquals("夜色渐深", state.blocks.first().text)
        assertEquals(2, state.blocks.size)
        assertEquals("dialogue", state.blocks[1].kind)
        assertEquals("「谁？」", state.blocks[1].text)
        assertTrue(state.thinkingSeconds != null)
    }

    @Test
    fun `polling reconciliation finds committed when stream ends without terminal`() = runBlocking {
        server.dispatcher = pollingDispatcher()
        val registry = registry()
        registry.ensureStreaming("s1", "b1", "t1", "c1", "测试故事")

        val state = withTimeout(10_000) {
            registry.state("t1").first { it.phase == TurnPhase.COMMITTED }
        }
        assertEquals("AB", state.blocks.first().text)
    }

    @Test
    fun `repeated disconnects do not fail and recovery still commits`() = runBlocking {
        // 失败次数超过旧实现的 5 次上限：旧逻辑会在此判 FAILED，新逻辑必须一直重连到终态。
        server.dispatcher = flakySseDispatcher(failures = 8)
        val registry = registry()
        registry.ensureStreaming("s1", "b1", "t1", "c1", "测试故事")

        val state = withTimeout(15_000) {
            registry.state("t1").first { it.phase == TurnPhase.COMMITTED }
        }
        assertEquals("夜色渐深", state.blocks.first().text)
    }

    @Test
    fun `terminal state notifies listener exactly once`() = runBlocking {
        server.dispatcher = sseDispatcher()
        val registry = registry()
        var notifications = 0
        registry.notifier = TurnNotifier { _, _ -> notifications++ }
        registry.ensureStreaming("s1", "b1", "t1", "c1", "测试故事")
        withTimeout(10_000) { registry.state("t1").first { it.phase == TurnPhase.COMMITTED } }
        delay(300)
        assertEquals(1, notifications)
    }
}
