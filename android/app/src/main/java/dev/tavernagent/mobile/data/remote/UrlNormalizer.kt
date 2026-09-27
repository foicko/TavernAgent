package dev.tavernagent.mobile.data.remote

import okhttp3.HttpUrl.Companion.toHttpUrlOrNull

/** 把用户输入整理成可用的 base URL：补协议、去尾部斜杠、保留端口。 */
object UrlNormalizer {

    fun normalize(raw: String): String {
        val trimmed = raw.trim().trimEnd('/')
        if (trimmed.isEmpty()) return ""
        val withScheme = when {
            trimmed.startsWith("http://", ignoreCase = true) -> trimmed
            trimmed.startsWith("https://", ignoreCase = true) -> trimmed
            else -> "http://$trimmed"
        }
        return withScheme
    }

    fun hostPort(baseUrl: String): Pair<String, Int>? {
        val url = baseUrl.toHttpUrlOrNull() ?: return null
        return url.host to url.port
    }

    fun isHttps(baseUrl: String): Boolean = baseUrl.startsWith("https://", ignoreCase = true)
}
