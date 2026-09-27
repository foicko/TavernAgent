package dev.tavernagent.mobile.domain

import java.text.Normalizer

/**
 * 卡内资源键 → URL 的解析规则（与 Web 端 `resolveCardAsset` 对齐，并额外支持 ref 与 Unicode 归一化）。
 *
 * 依次尝试（命中即返回）：
 *  1. 精确命中（原样；含 NFC 归一化后的形态）；
 *  2. 忽略大小写命中；
 *  3. 去掉图片扩展名后同名（`main` ↔ `main.png`、`robe.default` ↔ `robe.default.avif`）；
 *  4. 「纯数字后缀」变体（`dressed_happy` ↔ `dressed_happy1.png`、
 *     `anis_casual_surprised` ↔ `anis_casual_surprised3.webp`）；
 *  5. 资源 URL 末段（ref）同名——服务端会把非 ASCII 引用清洗成下划线，
 *     因此标记里直接写 ref 的情况也要能命中。
 *
 * 多张候选按键名排序后用键的字符和取模选取，保证同一键每次渲染稳定（不闪烁）。
 * 资源表来自卡预览的 `assets[]`（name → 服务端 URL），因此 [resolve] 负责把相对 URL 补成绝对地址。
 *
 * 批量渲染请先构建 [AssetIndex]（会话里同一张卡会出现几十个标记，逐次扫描会很浪费）。
 */
object CardAssetResolver {

    private val IMAGE_EXT = Regex("\\.(png|jpe?g|webp|gif|avif|bmp|svg|apng|jfif)$")

    /** 兼容入口：按资源表直接解析。 */
    fun resolve(assets: Map<String, String>, raw: String, baseUrl: String = ""): String? =
        resolveIndexed(index(assets), raw, baseUrl)

    fun index(assets: Map<String, String>): AssetIndex = AssetIndex.build(assets)

    /** 解析出可直接交给 Coil 的绝对 URL；`null` = 资源表里没有对应资源。 */
    fun resolveIndexed(index: AssetIndex, raw: String, baseUrl: String = ""): String? {
        val key = raw.trim()
        if (key.isEmpty()) return null
        // 键本身就是绝对地址或 data URI：原样返回。
        if (isAbsoluteUrl(key)) return key
        // 键是服务端路径（以 / 开头）：直接补全主机。
        if (key.startsWith("/")) return absolutize(key, baseUrl)
        val url = index.match(key) ?: return null
        return absolutize(url, baseUrl)
    }

    /** 把相对 URL（以 `/` 开头或纯路径）补成绝对地址；绝对地址与 data URI 原样返回。 */
    fun absolutize(url: String, baseUrl: String): String {
        val trimmed = url.trim()
        return when {
            isAbsoluteUrl(trimmed) -> trimmed
            baseUrl.isBlank() -> trimmed
            trimmed.startsWith("/") -> baseUrl.trimEnd('/') + trimmed
            else -> baseUrl.trimEnd('/') + "/" + trimmed
        }
    }

    private fun isAbsoluteUrl(url: String): Boolean =
        url.startsWith("http://", ignoreCase = true) ||
            url.startsWith("https://", ignoreCase = true) ||
            url.startsWith("data:", ignoreCase = true)

    private fun nfc(value: String): String =
        runCatching { Normalizer.normalize(value, Normalizer.Form.NFC) }.getOrDefault(value)

    private fun stripExtension(value: String): String = value.replace(IMAGE_EXT, "")

    /** 取资源 URL 的末段引用名（percent-decode，忽略查询串与锚点）。 */
    private fun refOf(url: String): String {
        val trimmed = url.trim()
        if (trimmed.isEmpty() || trimmed.startsWith("data:", ignoreCase = true)) return ""
        val path = trimmed.substringBefore('?').substringBefore('#')
        return percentDecode(path.substringAfterLast('/'))
    }

    /** JVM 侧的最小 percent-decoder（不依赖 android.net.Uri，单测可直接跑）。 */
    private fun percentDecode(value: String): String {
        if (value.indexOf('%') < 0) return value
        val out = java.io.ByteArrayOutputStream(value.length)
        var i = 0
        while (i < value.length) {
            val c = value[i]
            if (c == '%' && i + 3 <= value.length) {
                val byte = value.substring(i + 1, i + 3).toIntOrNull(16)
                if (byte != null) {
                    out.write(byte)
                    i += 3
                    continue
                }
            }
            out.write(c.toString().toByteArray(Charsets.UTF_8))
            i++
        }
        return String(out.toByteArray(), Charsets.UTF_8)
    }

    /** 预构建的查找表：会话渲染时按标记键做 O(1) 命中。 */
    class AssetIndex internal constructor(
        private val exact: Map<String, String>,
        private val lower: Map<String, String>,
        private val stems: Map<String, String>,
        private val numeric: Map<String, List<Pair<String, String>>>,
    ) {
        /** 资源表为空（尚未加载到，或该卡确实没有内嵌资源）。 */
        val isEmpty: Boolean
            get() = exact.isEmpty() && lower.isEmpty() && stems.isEmpty() && numeric.isEmpty()

        fun match(raw: String): String? {
            val key = raw.trim()
            if (key.isEmpty()) return null

            exact[key]?.let { return it }
            val keyNfc = nfc(key)
            if (keyNfc != key) exact[keyNfc]?.let { return it }

            val norm = keyNfc.lowercase()
            lower[norm]?.let { return it }

            val stem = stripExtension(norm)
            stems[stem]?.let { return it }

            val candidates = numeric[stem]
            if (!candidates.isNullOrEmpty()) {
                var hash = 0
                for (ch in key) hash = (hash + ch.code) % candidates.size
                return candidates[hash].second
            }
            return null
        }

        internal companion object {
            fun build(assets: Map<String, String>): AssetIndex {
                if (assets.isEmpty()) {
                    return AssetIndex(emptyMap(), emptyMap(), emptyMap(), emptyMap())
                }
                val exact = HashMap<String, String>(assets.size * 2)
                val lower = HashMap<String, String>(assets.size * 2)
                val stems = HashMap<String, String>(assets.size * 2)
                val numeric = HashMap<String, MutableList<Pair<String, String>>>()

                fun addIdentity(value: String, url: String) {
                    val trimmed = value.trim()
                    if (trimmed.isEmpty()) return
                    exact.putIfAbsent(trimmed, url)
                    val valueNfc = nfc(trimmed)
                    if (valueNfc != trimmed) exact.putIfAbsent(valueNfc, url)

                    val norm = valueNfc.lowercase()
                    lower.putIfAbsent(norm, url)

                    val stem = stripExtension(norm)
                    if (stem.isEmpty()) return
                    stems.putIfAbsent(stem, url)
                    // 数字后缀变体：stem = 前缀 + 纯数字，则前缀也能命中（与 Web 端一致）。
                    for (i in 1 until stem.length) {
                        val tail = stem.substring(i)
                        if (tail.all { it.isDigit() }) {
                            numeric.getOrPut(stem.substring(0, i)) { mutableListOf() }.add(stem to url)
                        }
                    }
                }

                // 名称优先于 ref：先登记全部 name，再补充 URL 末段引用名。
                for ((name, url) in assets) {
                    if (url.isNotBlank()) addIdentity(name, url)
                }
                for ((_, url) in assets) {
                    if (url.isBlank()) continue
                    val ref = refOf(url)
                    if (ref.isNotEmpty()) addIdentity(ref, url)
                }

                val stableNumeric = numeric.mapValues { (_, list) ->
                    list.distinctBy { it.first }.sortedBy { it.first }
                }
                return AssetIndex(exact, lower, stems, stableNumeric)
            }
        }
    }
}
