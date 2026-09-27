package dev.tavernagent.mobile.domain

import java.util.regex.Pattern

/**
 * 正文内容切片：文本、多模态图片、场景提示、HUD 状态面板、思考引文。
 */
sealed interface ContentSegment {
    data class Text(val content: String) : ContentSegment
    data class Image(val url: String, val alt: String = "", val rawKey: String = "") : ContentSegment
    data class Scene(val title: String) : ContentSegment
    data class Hud(val values: Map<String, String>) : ContentSegment
    data class Thought(val thought: String) : ContentSegment
}

/**
 * 卡片 UI 标记与多模态解析器。
 *
 * 语法规范（与 internal/cardmarkers 及 web/src/lib/cardMarkers.tsx 严格对齐）：
 * - <img cmd="key"> / <img="key"> / <img_obj="key"> / <imgBG="key"> / <img src="key">
 * - [IMG:key]
 * - Markdown 图片：![alt](url)
 * - [HUD:k=v|k=v]
 * - [SCENE:title]
 * - [VAR:k=v]（会话变量，正文剥离）
 * - <thought>...</thought> 思考块
 * - 转义：[[ 转为字面量 [
 */
object CardMarkerParser {

    private const val ESCAPE_SENTINEL = "\u0000LBRACK\u0000"

    private val THOUGHT_PATTERN = Pattern.compile("(?is)<thought>(.*?)</thought>")
    private val MARKER_PATTERN = Pattern.compile(
        """(?i)<img\s*[-\w]*\s*=\s*"([^"]{0,400})"[^>]*>""" +
            """|\[IMG:([^\[\]\r\n]{0,400})\]""" +
            """|!\[([^\]]*)\]\(([^)\s]+)[^)]*\)""" +
            """|\[HUD:([^\[\]\r\n]{0,400})\]""" +
            """|\[SCENE:([^\[\]\r\n]{0,400})\]""" +
            """|\[VAR:([^\[\]\r\n]{0,400})\]"""
    )

    fun parse(rawText: String, resolveAsset: (String) -> String?): List<ContentSegment> {
        if (rawText.isBlank()) return emptyList()

        val segments = mutableListOf<ContentSegment>()

        // 1. 优先提取显式 <thought> 标签（常由部分推理模型输出）
        val thoughtMatcher = THOUGHT_PATTERN.matcher(rawText)
        var lastThoughtEnd = 0

        while (thoughtMatcher.find()) {
            val start = thoughtMatcher.start()
            val end = thoughtMatcher.end()

            if (start > lastThoughtEnd) {
                val beforeText = rawText.substring(lastThoughtEnd, start)
                segments.addAll(parseBodySegments(beforeText, resolveAsset))
            }

            val thoughtContent = thoughtMatcher.group(1)?.trim().orEmpty()
            if (thoughtContent.isNotBlank()) {
                segments.add(ContentSegment.Thought(thoughtContent))
            }
            lastThoughtEnd = end
        }

        if (lastThoughtEnd < rawText.length) {
            val remainingText = rawText.substring(lastThoughtEnd)
            segments.addAll(parseBodySegments(remainingText, resolveAsset))
        }

        // 合并连续文本块并清理剥离残留空白
        return mergeAdjacentTexts(segments)
    }

    private fun parseBodySegments(text: String, resolveAsset: (String) -> String?): List<ContentSegment> {
        if (!text.contains("[") && !text.contains("<") && !text.contains("!")) {
            return listOf(ContentSegment.Text(text))
        }

        val protectedText = text.replace("[[", ESCAPE_SENTINEL)
        val matcher = MARKER_PATTERN.matcher(protectedText)
        val result = mutableListOf<ContentSegment>()
        var last = 0

        fun pushText(s: String) {
            if (s.isEmpty()) return
            val restored = s.replace(ESCAPE_SENTINEL, "[")
            result.add(ContentSegment.Text(restored))
        }

        while (matcher.find()) {
            val start = matcher.start()
            if (start > last) {
                pushText(protectedText.substring(last, start))
            }

            val imgTagKey = matcher.group(1)
            val imgBracketKey = matcher.group(2)
            val mdImgAlt = matcher.group(3)
            val mdImgUrl = matcher.group(4)
            val hudBody = matcher.group(5)
            val sceneBody = matcher.group(6)
            // group(7) is VAR

            when {
                // 1. <img ...="key">
                imgTagKey != null -> {
                    val key = imgTagKey.trim()
                    // 解析不到也保留为图片段（空 URL）：由渲染端给出缺失提示，
                    // 而不是把裸标记当正文显示、或让调用方去拼一个必然 404 的地址。
                    result.add(ContentSegment.Image(url = resolveAsset(key).orEmpty(), alt = key, rawKey = key))
                }
                // 2. [IMG:key]
                imgBracketKey != null -> {
                    val key = imgBracketKey.trim()
                    result.add(ContentSegment.Image(url = resolveAsset(key).orEmpty(), alt = key, rawKey = key))
                }
                // 3. ![alt](url)
                mdImgUrl != null -> {
                    val url = mdImgUrl.trim()
                    val resolved = if (url.startsWith("http://") || url.startsWith("https://") || url.startsWith("data:")) {
                        url
                    } else {
                        resolveAsset(url) ?: url
                    }
                    result.add(ContentSegment.Image(url = resolved, alt = mdImgAlt.orEmpty().trim(), rawKey = url))
                }
                // 4. [HUD:k=v|...]
                hudBody != null -> {
                    val pairs = parsePairs(hudBody)
                    if (pairs.isNotEmpty()) {
                        result.add(ContentSegment.Hud(pairs))
                    } else {
                        pushText(matcher.group(0).orEmpty())
                    }
                }
                // 5. [SCENE:title]
                sceneBody != null -> {
                    val title = sceneBody.trim()
                    if (title.isNotEmpty()) {
                        result.add(ContentSegment.Scene(title))
                    } else {
                        pushText(matcher.group(0).orEmpty())
                    }
                }
                // 6. [VAR:...] 会话变量静默从正文中剥离，不输出文本
            }

            last = matcher.end()
        }

        if (last < protectedText.length) {
            pushText(protectedText.substring(last))
        }

        return result
    }

    private fun parsePairs(body: String): Map<String, String> {
        val out = mutableMapOf<String, String>()
        for (part in body.split('|')) {
            val eq = part.indexOf('=')
            if (eq < 0) continue
            val k = part.substring(0, eq).trim()
            val v = part.substring(eq + 1).trim()
            if (k.isNotEmpty()) out[k] = v
        }
        return out
    }

    private fun mergeAdjacentTexts(segments: List<ContentSegment>): List<ContentSegment> {
        if (segments.size <= 1) return segments
        val merged = mutableListOf<ContentSegment>()
        var currentText: StringBuilder? = null

        for (seg in segments) {
            if (seg is ContentSegment.Text) {
                if (currentText == null) {
                    currentText = StringBuilder(seg.content)
                } else {
                    currentText.append(seg.content)
                }
            } else {
                currentText?.let {
                    val t = tidyText(it.toString())
                    if (t.isNotEmpty()) merged.add(ContentSegment.Text(t))
                    currentText = null
                }
                merged.add(seg)
            }
        }
        currentText?.let {
            val t = tidyText(it.toString())
            if (t.isNotEmpty()) merged.add(ContentSegment.Text(t))
        }
        return merged
    }

    private fun tidyText(text: String): String {
        // 压缩因变量剥离产生的连续多个空格为单个空格
        return text.replace(Regex("[ \\t]{2,}"), " ")
    }
}
