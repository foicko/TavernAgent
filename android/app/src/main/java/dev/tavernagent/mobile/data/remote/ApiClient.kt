package dev.tavernagent.mobile.data.remote

import dev.tavernagent.mobile.data.local.ServerCredentials
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlin.random.Random

/**
 * 一次会话（一组连接凭据）对应的 HTTP 客户端。
 *
 * 约定：
 * - 所有写请求自动带 `Authorization: Bearer <token>`（Token 为空时不带，本机免检）。
 * - 自动带 `X-Request-ID`，服务端会在错误体 traceId 中回显，便于对日志。
 * - 非 2xx 统一抛出 [ApiException]；网络层异常统一抛 [NetworkFailure]。
 */
class ApiClient(
    val credentials: ServerCredentials,
    private val json: Json = ApiJson.instance,
) {

    private val baseUrl: HttpUrl = credentials.baseUrl.toHttpUrlOrNull()
        ?: throw NetworkFailure("服务地址无效：${credentials.baseUrl}", null)

    private val authHeader: String? = credentials.token.takeIf { it.isNotBlank() }?.let { "Bearer $it" }

    private val tls: HttpEngine.TrustedTls? by lazy { credentials.trustedCertDer()?.let { HttpEngine.tlsFor(it) } }

    private val regular: OkHttpClient by lazy {
        val builder = OkHttpClient.Builder()
            .connectTimeout(10, TimeUnit.SECONDS)
            .writeTimeout(120, TimeUnit.SECONDS)
            .readTimeout(60, TimeUnit.SECONDS)
            .retryOnConnectionFailure(true)
        tls?.let { builder.sslSocketFactory(it.socketFactory, it.trustManager) }
        builder.build()
    }

    private val streaming: OkHttpClient by lazy {
        val builder = OkHttpClient.Builder()
            .connectTimeout(10, TimeUnit.SECONDS)
            .writeTimeout(30, TimeUnit.SECONDS)
            .readTimeout(0, TimeUnit.MILLISECONDS)
            .retryOnConnectionFailure(false)
        tls?.let { builder.sslSocketFactory(it.socketFactory, it.trustManager) }
        builder.build()
    }

    fun url(path: String, query: Map<String, String> = emptyMap()): HttpUrl {
        val builder = baseUrl.newBuilder()
        path.trimStart('/').split('/').forEach { segment -> builder.addPathSegment(segment) }
        query.forEach { (key, value) -> builder.addQueryParameter(key, value) }
        return builder.build()
    }

    suspend fun get(path: String, query: Map<String, String> = emptyMap()): JsonElement =
        execute("GET", url(path, query), null, null)

    suspend fun post(path: String, body: JsonElement?, timeoutSeconds: Long? = null): JsonElement =
        execute("POST", url(path), body?.toString(), timeoutSeconds)

    suspend fun put(path: String, body: JsonElement?, timeoutSeconds: Long? = null): JsonElement =
        execute("PUT", url(path), body?.toString(), timeoutSeconds)

    suspend fun patch(path: String, body: JsonElement?): JsonElement =
        execute("PATCH", url(path), body?.toString(), null)

    suspend fun delete(path: String): JsonElement = execute("DELETE", url(path), null, null)

    /** 上传原始字节（角色卡导入：PNG / JSON / charx 归档）。 */
    suspend fun upload(path: String, bytes: ByteArray, mediaType: String, timeoutSeconds: Long = 300): JsonElement =
        withContext(Dispatchers.IO) {
            executeRaw("POST", url(path), bytes.toRequestBody(mediaType.toMediaType()), timeoutSeconds)
        }

    /** 下载二进制（故事包导出）。 */
    suspend fun downloadBytes(path: String, query: Map<String, String> = emptyMap()): ByteArray =
        withContext(Dispatchers.IO) { executeRawBytes("GET", url(path, query), null, 600) }

    /** 请求并读取二进制响应（TTS 合成）。 */
    suspend fun postForBytes(path: String, jsonBody: String?, timeoutSeconds: Long = 120): ByteArray =
        withContext(Dispatchers.IO) {
            executeRawBytes(
                "POST",
                url(path),
                (jsonBody ?: "").toRequestBody(JSON_MEDIA_TYPE),
                timeoutSeconds,
                accept = "audio/mpeg, application/octet-stream",
            )
        }

    /** 走默认 JSON 解析；空响应体返回 [JsonNull]。 */
    private suspend fun execute(method: String, url: HttpUrl, jsonBody: String?, timeoutSeconds: Long?): JsonElement {
        val body: RequestBody? = when (method) {
            "POST", "PUT", "PATCH" -> (jsonBody ?: "").toRequestBody(JSON_MEDIA_TYPE)
            else -> null
        }
        return withContext(Dispatchers.IO) { executeRaw(method, url, body, timeoutSeconds) }
    }

    private suspend fun executeRaw(
        method: String,
        url: HttpUrl,
        body: RequestBody?,
        timeoutSeconds: Long?,
    ): JsonElement = withContext(Dispatchers.IO) {
        val request = Request.Builder()
            .url(url)
            .method(method, body)
            .header("Accept", "application/json")
            .applyIdempotencyAndAuth()
            .build()
        val call = regular.newCall(request)
        timeoutSeconds?.let { call.timeout().timeout(it, TimeUnit.SECONDS) }
        try {
            call.execute().use { response ->
                val raw = response.body?.string()
                if (!response.isSuccessful) {
                    throw ApiErrorParser.from(response.code, raw, response.message)
                }
                if (raw.isNullOrBlank()) JsonNull else parseJson(raw)
            }
        } catch (e: ApiException) {
            throw e
        } catch (e: IOException) {
            throw mapIoException(e)
        }
    }

    private suspend fun executeRawBytes(
        method: String,
        url: HttpUrl,
        body: RequestBody?,
        timeoutSeconds: Long?,
        accept: String = "*/*",
    ): ByteArray = withContext(Dispatchers.IO) {
        val request = Request.Builder()
            .url(url)
            .method(method, body)
            .header("Accept", accept)
            .applyIdempotencyAndAuth()
            .build()
        val call = regular.newCall(request)
        timeoutSeconds?.let { call.timeout().timeout(it, TimeUnit.SECONDS) }
        try {
            call.execute().use { response ->
                val bytes = response.body?.bytes()
                if (!response.isSuccessful) {
                    val raw = bytes?.toString(Charsets.UTF_8)
                    throw ApiErrorParser.from(response.code, raw, response.message)
                }
                bytes ?: ByteArray(0)
            }
        } catch (e: ApiException) {
            throw e
        } catch (e: IOException) {
            throw mapIoException(e)
        }
    }

    /** 打开一条 SSE 流。请求失败（非 2xx）时，流在收集时抛出 [ApiException]。 */
    fun stream(path: String, query: Map<String, String> = emptyMap(), lastEventId: String? = null): Flow<SseFrame> {
        val builder = Request.Builder()
            .url(url(path, query))
            .get()
            .header("Accept", "text/event-stream")
            .applyIdempotencyAndAuth()
        if (!lastEventId.isNullOrBlank()) builder.header("Last-Event-ID", lastEventId)
        return SseClient(streaming).stream(builder.build())
    }

    private fun Request.Builder.applyIdempotencyAndAuth(): Request.Builder {
        header("X-Request-ID", randomRequestId())
        authHeader?.let { header("Authorization", it) }
        return this
    }

    private fun parseJson(raw: String): JsonElement =
        try {
            json.parseToJsonElement(raw)
        } catch (e: Exception) {
            throw NetworkFailure("服务端返回了无法解析的内容", e)
        }

    private fun mapIoException(e: IOException): IOException = when (e) {
        is javax.net.ssl.SSLHandshakeException,
        is javax.net.ssl.SSLPeerUnverifiedException,
        -> NetworkFailure("TLS 证书未受信任或校验失败", e)
        else -> NetworkFailure("网络连接失败：${e.message ?: e.javaClass.simpleName}", e)
    }

    private fun randomRequestId(): String = buildString {
        repeat(16) { append(HEX[Random.nextInt(HEX.length)]) }
    }

    // ---- 按域分组的 API 视图（懒建，共享连接池）----

    val auth by lazy { dev.tavernagent.mobile.data.remote.api.AuthApi(this) }
    val cards by lazy { dev.tavernagent.mobile.data.remote.api.CardsApi(this) }
    val sessions by lazy { dev.tavernagent.mobile.data.remote.api.SessionsApi(this) }
    val turns by lazy { dev.tavernagent.mobile.data.remote.api.TurnsApi(this) }
    val memories by lazy { dev.tavernagent.mobile.data.remote.api.MemoriesApi(this) }
    val director by lazy { dev.tavernagent.mobile.data.remote.api.DirectorApi(this) }
    val models by lazy { dev.tavernagent.mobile.data.remote.api.ModelsApi(this) }
    val media by lazy { dev.tavernagent.mobile.data.remote.api.MediaApi(this) }

    companion object {
        private val JSON_MEDIA_TYPE = "application/json; charset=utf-8".toMediaType()
        private val HEX = "0123456789abcdef"
    }
}
