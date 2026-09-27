package dev.tavernagent.mobile.data.remote

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.io.IOException

/** 后端统一错误体：{"code","message","retryable","turnId","traceId"}。 */
class ApiException(
    val status: Int,
    val code: String,
    override val message: String,
    val retryable: Boolean = false,
    val turnId: String = "",
    val traceId: String = "",
    cause: Throwable? = null,
) : IOException(message, cause) {

    /** 局域网鉴权失败：需要（重新）配对。 */
    val needsPairing: Boolean get() = status == 401 && (code == "AUTH_REQUIRED" || code == "INVALID_PIN")

    /** 被局域网来源规则拒绝：地址不是本机/私有网段。 */
    val isLanForbidden: Boolean get() = status == 403

    val isRateLimited: Boolean get() = status == 429
}

/** 传输层失败（断网、超时、连接被拒）。 */
class NetworkFailure(message: String, cause: Throwable?) : IOException(message, cause)

class SslTrustRequired(val host: String, val port: Int, val fingerprintSha256: String) :
    IOException("服务器证书需要确认：$fingerprintSha256")

/** 把 HTTP 状态与响应体映射为 [ApiException]。响应体不是 JSON 错误体时退化为 HTTP 语义。 */
object ApiErrorParser {

    private val json = Json { ignoreUnknownKeys = true; isLenient = true }

    fun from(status: Int, rawBody: String?, responseMessage: String?): ApiException {
        val parsed = parse(rawBody)
        if (parsed != null) {
            return ApiException(
                status = status,
                code = parsed.stringOrNull("code") ?: fallbackCode(status),
                message = parsed.stringOrNull("message") ?: (responseMessage ?: fallbackMessage(status)),
                retryable = parsed["retryable"]?.jsonPrimitive?.content?.toBooleanStrictOrNull() ?: false,
                turnId = parsed.stringOrNull("turnId").orEmpty(),
                traceId = parsed.stringOrNull("traceId").orEmpty(),
            )
        }
        return ApiException(status, fallbackCode(status), responseMessage ?: fallbackMessage(status))
    }

    /** SSE 错误事件与断流错误共用同一套字段。 */
    fun fromEventData(rawBody: String?): ApiException? {
        val parsed = parse(rawBody) ?: return null
        val code = parsed.stringOrNull("code") ?: return null
        return ApiException(
            status = 0,
            code = code,
            message = parsed.stringOrNull("message") ?: "生成失败",
            retryable = parsed["retryable"]?.jsonPrimitive?.content?.toBooleanStrictOrNull() ?: false,
            turnId = parsed.stringOrNull("turnId").orEmpty(),
            traceId = parsed.stringOrNull("traceId").orEmpty(),
        )
    }

    private fun parse(raw: String?): JsonObject? {
        if (raw.isNullOrBlank()) return null
        return try {
            json.parseToJsonElement(raw).jsonObject
        } catch (_: Exception) {
            null
        }
    }

    private fun JsonObject.stringOrNull(key: String): String? =
        this[key]?.let { runCatching { it.jsonPrimitive.content }.getOrNull() }

    private fun fallbackCode(status: Int): String = when (status) {
        400 -> "BAD_REQUEST"
        401 -> "AUTH_REQUIRED"
        403 -> "FORBIDDEN"
        404 -> "NOT_FOUND"
        409 -> "CONFLICT"
        413 -> "REQUEST_TOO_LARGE"
        429 -> "TOO_MANY_ATTEMPTS"
        500 -> "INTERNAL"
        else -> "HTTP_$status"
    }

    private fun fallbackMessage(status: Int): String = when (status) {
        401 -> "需要配对授权"
        403 -> "服务端拒绝了该请求（地址需为本机或私有局域网，写请求来源需合法）"
        404 -> "资源不存在"
        409 -> "状态已变化，请刷新后重试"
        413 -> "请求内容过大"
        429 -> "尝试次数过多，请稍后重试"
        500 -> "服务内部错误，请稍后重试"
        else -> "请求失败（HTTP $status）"
    }
}

/** 便于把任意 JSON 元素安全地转成字符串。 */
fun JsonElement.contentOrNull(): String? = runCatching { jsonPrimitive.content }.getOrNull()
