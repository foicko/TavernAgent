package dev.tavernagent.mobile.ui.components

import java.time.Duration
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.ZonedDateTime
import java.time.format.DateTimeFormatter
import java.time.format.TextStyle
import java.util.Locale

/**
 * 聊天语义的相对时间：刚刚 / N 分钟前 / HH:mm / 昨天 / 周几 / MM-dd。
 * 解析失败时退化为原字符串的前 16 个字符（服务端时间戳不可解析时也不崩）。
 */
fun relativeTime(raw: String?): String {
    if (raw.isNullOrBlank()) return ""
    val time = runCatching {
        OffsetDateTime.parse(raw).atZoneSameInstant(ZoneId.systemDefault())
    }.getOrNull() ?: return raw.take(16).replace('T', ' ')

    val now = ZonedDateTime.now()
    val minutes = Duration.between(time, now).toMinutes()
    return when {
        minutes < 1 -> "刚刚"
        minutes < 60 -> "${minutes}分钟前"
        time.toLocalDate() == now.toLocalDate() -> time.format(DateTimeFormatter.ofPattern("HH:mm"))
        time.toLocalDate() == now.toLocalDate().minusDays(1) -> "昨天"
        minutes < 60 * 24 * 6 -> time.dayOfWeek.getDisplayName(TextStyle.SHORT, Locale.getDefault())
        else -> time.format(DateTimeFormatter.ofPattern("MM-dd"))
    }
}
