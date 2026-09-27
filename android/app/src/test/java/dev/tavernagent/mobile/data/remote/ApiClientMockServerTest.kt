package dev.tavernagent.mobile.data.remote

import dev.tavernagent.mobile.data.local.ServerCredentials
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.util.concurrent.TimeUnit

/**
 * 客户端 ⇄ 服务端契约的集成测试（真实 OkHttp + MockWebServer）。
 * 覆盖关键流程：配对、导入卡、开新故事、发送回合、SSE 流式、分支、导出。
 */
class ApiClientMockServerTest {

    private lateinit var server: MockWebServer
    private lateinit var client: ApiClient

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        client = ApiClient(ServerCredentials(baseUrl = server.url("/").toString().trimEnd('/')))
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    private fun json(body: String) = MockResponse()
        .setResponseCode(200)
        .setHeader("Content-Type", "application/json; charset=utf-8")
        .setBody(body)

    private fun takeRequest(): RecordedRequest = server.takeRequest(5, TimeUnit.SECONDS)!!

    @Test
    fun `pairing posts pin and stores returned token`() = runBlocking {
        server.enqueue(json("""{"ok":true,"token":"tok-123"}"""))
        val result = client.auth.pair("654321")
        assertTrue(result.ok)
        assertEquals("tok-123", result.token)

        val request = takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/api/v1/auth/pair", request.path)
        assertEquals("""{"pin":"654321"}""", request.body.readUtf8())
    }

    @Test
    fun `error envelope maps to ApiException with pairing hint`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setResponseCode(401)
                .setHeader("Content-Type", "application/json")
                .setBody("""{"code":"AUTH_REQUIRED","message":"局域网访问需要配对码授权","retryable":false,"turnId":"","traceId":"abc"}"""),
        )
        val error = runCatching { client.auth.status() }.exceptionOrNull()
        assertTrue(error is ApiException)
        error as ApiException
        assertEquals("AUTH_REQUIRED", error.code)
        assertTrue(error.needsPairing)
        assertEquals(401, error.status)
    }

    @Test
    fun `forwarded request carries bearer token and request id`() = runBlocking {
        server.enqueue(json("""{"authenticated":true,"required":true}"""))
        val authed = ApiClient(
            ServerCredentials(baseUrl = server.url("/").toString().trimEnd('/'), token = "tok-abc"),
        )
        authed.auth.status()
        val request = takeRequest()
        assertEquals("Bearer tok-abc", request.getHeader("Authorization"))
        assertTrue(!request.getHeader("X-Request-ID").isNullOrBlank())
    }

    @Test
    fun `card import uploads raw bytes and parses preview`() = runBlocking {
        server.enqueue(
            json(
                """{"cardId":"card_1","name":"酒馆老板娘","role":"店主","format":"native",
                    "openings":[{"variantId":"v1","title":"默认","text":"门开了。"}],
                    "assets":[{"name":"立绘","type":"image","url":"/api/v1/cards/card_1/assets/body"}]}""",
            ),
        )
        val bytes = byteArrayOf(0x89.toByte(), 'P'.code.toByte(), 'N'.code.toByte(), 'G'.code.toByte())
        val preview = client.cards.import(bytes, "image/png")
        assertEquals("card_1", preview.cardId)
        assertEquals("酒馆老板娘", preview.name)
        assertEquals("v1", preview.openings.first().variantId)

        val request = takeRequest()
        assertEquals("/api/v1/cards", request.path)
        assertEquals("image/png", request.getHeader("Content-Type"))
    }

    @Test
    fun `create session sends idempotency key and card id`() = runBlocking {
        server.enqueue(
            json(
                """{"sessionId":"s1","title":"雨夜","rootNodeId":"r1","branchId":"b1",
                    "branchName":"main","branchVersion":0,"openingText":"雨落在窗上。","state":{}}""",
            ),
        )
        val created = client.sessions.create(
            dev.tavernagent.mobile.data.remote.api.CreateSessionBodyDto(
                idempotencyKey = "cs_1",
                title = "雨夜",
                cardId = "card_1",
                playerName = "旅人",
            ),
        )
        assertEquals("s1", created.sessionId)
        assertEquals("b1", created.branchId)
        val body = takeRequest().body.readUtf8()
        assertTrue(body.contains("\"idempotencyKey\":\"cs_1\""))
        assertTrue(body.contains("\"cardId\":\"card_1\""))
        assertTrue(body.contains("\"playerName\":\"旅人\""))
    }

    @Test
    fun `accept turn sends optimistic concurrency fields and input`() = runBlocking {
        server.enqueue(json("""{"turnId":"t1","status":"generating","eventsUrl":"/api/v1/turns/t1/events"}"""))
        val response = client.turns.accept(
            "s1",
            "b1",
            dev.tavernagent.mobile.data.remote.api.AcceptTurnBodyDto(
                idempotencyKey = "turn_9",
                expectedHeadId = "n7",
                expectedVersion = 12,
                input = dev.tavernagent.mobile.data.remote.api.TurnInputDto(
                    kind = "text",
                    text = "我推门进去",
                    note = "放慢节奏",
                    options = "always",
                ),
                expectedCharacterId = "card_1",
            ),
        )
        assertEquals("t1", response.turnId)
        val body = takeRequest().body.readUtf8()
        assertTrue(body.contains("\"expectedHeadId\":\"n7\""))
        assertTrue(body.contains("\"expectedVersion\":12"))
        assertTrue(body.contains("\"note\":\"放慢节奏\""))
        assertTrue(body.contains("\"options\":\"always\""))
        assertTrue(body.contains("\"expectedCharacterId\":\"card_1\""))
    }

    @Test
    fun `turn event stream emits every frame until terminal`() = runBlocking {
        val sse = buildString {
            append("event: turn.started\ndata: {\"turnId\":\"t1\",\"branchId\":\"b1\"}\n\n")
            append(": ping\n\n")
            append("event: turn.thinking\ndata: {\"thinking\":\"先稳住\"}\n\n")
            append("event: block.delta\ndata: {\"seq\":1,\"kind\":\"narration\",\"delta\":\"门轴\"}\n\n")
            append("event: block.delta\ndata: {\"seq\":1,\"kind\":\"narration\",\"delta\":\"轻响\"}\n\n")
            append("event: turn.committed\nid: t1:3\ndata: {\"nodeId\":\"n8\"}\n\n")
        }
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "text/event-stream")
                .setBody(sse),
        )
        val frames = mutableListOf<SseFrame>()
        client.turns.events("t1").collect { frames.add(it) }
        assertEquals(5, frames.size)
        val events = frames.mapNotNull { TurnEventCodecShim.parse(it) }
        assertTrue(events.contains(dev.tavernagent.mobile.domain.TurnEvent.ThinkingDelta("先稳住")))
        assertTrue(events.contains(dev.tavernagent.mobile.domain.TurnEvent.Committed("n8")))
        assertEquals("t1:3", frames.last().id)
    }

    @Test
    fun `fork branch posts from node and parses branch`() = runBlocking {
        server.enqueue(json("""{"branch":{"branchId":"b2","name":"平行分支_1","headNodeId":"n3","version":0}}"""))
        val branch = client.sessions.fork("s1", "n3", "平行分支_1", "card_1")
        assertEquals("b2", branch.branchId)
        val body = takeRequest().body.readUtf8()
        assertTrue(body.contains("\"fromNodeId\":\"n3\""))
        assertTrue(body.contains("\"expectedCharacterId\":\"card_1\""))
    }

    @Test
    fun `export downloads zip bytes`() = runBlocking {
        val payload = byteArrayOf(0x50, 0x4B, 0x03, 0x04, 1, 2, 3)
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/zip")
                .setBody(okio.Buffer().write(payload)),
        )
        val bytes = client.sessions.exportBytes("s1", null)
        assertEquals(payload.toList(), bytes.toList())
        assertEquals("/api/v1/sessions/s1/export", takeRequest().path)
    }

    @Test
    fun `memory patch is branch scoped copy on write`() = runBlocking {
        server.enqueue(json("""{"memoryId":"m2","content":"修订后的内容","effective":true,"protected":false}"""))
        client.memories.patch(
            "s1",
            "b1",
            "m1",
            dev.tavernagent.mobile.data.remote.dto.MemoryPatchBody(
                content = "修订后的内容",
                expectedCharacterId = "card_1",
                expectedHeadId = "n7",
                expectedVersion = 5,
                idempotencyKey = "mp_1",
            ),
        )
        val request = takeRequest()
        assertEquals("/api/v1/sessions/s1/branches/b1/memories/m1", request.path)
        assertEquals("PATCH", request.method)
        val body = request.body.readUtf8()
        assertTrue(body.contains("\"expectedVersion\":5"))
        assertTrue(body.contains("\"idempotencyKey\":\"mp_1\""))
    }
}

/** 测试里直接复用生产解析器。 */
private object TurnEventCodecShim {
    fun parse(frame: SseFrame) = dev.tavernagent.mobile.domain.TurnEventCodec.parse(frame)
}
