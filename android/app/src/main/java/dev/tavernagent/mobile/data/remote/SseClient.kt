package dev.tavernagent.mobile.data.remote

import kotlinx.coroutines.Job
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.flowOn
import kotlinx.coroutines.Dispatchers
import okhttp3.Call
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response

/** 一帧 SSE：注释（心跳）不会作为帧出现；[id] 形如 "aggregate:sequence"。
 *
 * [retryMillis] 是服务端通过 `retry:` 字段给出的重连间隔建议（毫秒），未给出时为 null。
 * 它是**流级**设置而非单帧内容：一旦出现，后续所有帧都带上该值。 */
data class SseFrame(
    val event: String,
    val data: String,
    val id: String?,
    val retryMillis: Long? = null,
)

/**
 * 解析 text/event-stream。规则遵循 SSE 规范：
 * 冒号开头为注释（`:` 心跳）；空行派发；`event`/`data`/`id`/`retry` 字段；
 * data 值去掉一个前导空格；多行 data 用 \n 连接。
 *
 * 两处此前偏离规范：`event:` 的空值应视同"未指定事件名"，派发时回落为 `message`
 * （否则会静默产出 event="" 的帧被消费端丢弃）；`retry:` 应记录为流级重连间隔
 * 并交给重连方使用（此前直接忽略）。
 */
class SseLineParser {
    private val data = StringBuilder()
    private var event: String? = null
    private var id: String? = null
    private var retryMillis: Long? = null

    fun accept(line: String): SseFrame? {
        if (line.isEmpty()) return dispatch()
        if (line[0] == ':') return null
        val colon = line.indexOf(':')
        val field = if (colon < 0) line else line.substring(0, colon)
        var value = if (colon < 0) "" else line.substring(colon + 1)
        if (value.startsWith(" ")) value = value.substring(1)
        when (field) {
            // 空事件名按规范回落为 message（见 dispatch 的 `event ?: "message"`）。
            "event" -> event = value.ifEmpty { null }
            "data" -> {
                if (data.isNotEmpty()) data.append('\n')
                data.append(value)
            }
            "id" -> id = value
            // 只接受纯数字（规范要求），并作为流级重连间隔保留。
            "retry" -> value.toLongOrNull()?.takeIf { it >= 0 }?.let { retryMillis = it }
        }
        return null
    }

    fun endOfStream(): SseFrame? = dispatch()

    private fun dispatch(): SseFrame? {
        if (data.isEmpty()) {
            event = null
            id = null
            return null
        }
        val frame = SseFrame(
            event = event ?: "message",
            data = data.toString(),
            id = id,
            retryMillis = retryMillis,
        )
        data.setLength(0)
        event = null
        id = null
        return frame
    }
}

/**
 * 基于 OkHttp 的 SSE 客户端。
 *
 * 取消语义：收集方取消协程时通过 `Job.invokeOnCompletion` 取消底层 [Call]，
 * 阻塞的 socket 读会立刻抛错退出，不需要等服务器下一次写入。
 */
class SseClient(private val client: OkHttpClient) {

    fun stream(request: Request): Flow<SseFrame> = flow {
        val call = client.newCall(request)
        val job = currentCoroutineContext()[Job]
        val handle = job?.invokeOnCompletion { call.cancel() }
        try {
            val response = try {
                call.execute()
            } catch (e: java.io.IOException) {
                throw NetworkFailure("连接中断：${e.message ?: e.javaClass.simpleName}", e)
            }
            response.use { resp ->
                if (!resp.isSuccessful) {
                    throw ApiErrorParser.from(resp.code, resp.body?.string(), resp.message)
                }
                val source = resp.body?.source()
                    ?: throw NetworkFailure("响应没有内容", null)
                val parser = SseLineParser()
                while (true) {
                    val line = source.readUtf8Line() ?: break
                    parser.accept(line)?.let { emit(it) }
                }
                parser.endOfStream()?.let { emit(it) }
            }
        } finally {
            handle?.dispose()
        }
    }.flowOn(Dispatchers.IO)
}
