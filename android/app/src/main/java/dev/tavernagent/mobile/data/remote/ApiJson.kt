package dev.tavernagent.mobile.data.remote

import kotlinx.serialization.json.Json
import kotlinx.serialization.serializer

/** 全局 JSON 配置：后端字段会随版本增加，客户端必须容忍未知字段。 */
object ApiJson {
    val instance: Json = Json {
        ignoreUnknownKeys = true
        isLenient = true
        explicitNulls = false
        encodeDefaults = true
        coerceInputValues = true
    }

    fun encode(element: kotlinx.serialization.json.JsonElement): String =
        instance.encodeToString(kotlinx.serialization.json.JsonElement.serializer(), element)

    inline fun <reified T> decodeOrNull(raw: String?): T? {
        if (raw.isNullOrBlank()) return null
        return runCatching { instance.decodeFromString<T>(raw) }.getOrNull()
    }

    /** 从已经解析好的 JSON 元素反序列化。 */
    inline fun <reified T> from(element: kotlinx.serialization.json.JsonElement): T =
        instance.decodeFromJsonElement(serializer<T>(), element)

    /** 把任意 @Serializable 值编码为 JSON 元素。 */
    inline fun <reified T> toJson(value: T): kotlinx.serialization.json.JsonElement =
        instance.encodeToJsonElement(serializer<T>(), value)
}
