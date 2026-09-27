package dev.tavernagent.mobile.domain

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.text.Normalizer

/**
 * 会话图片解析的回归用例，全部取自真实数据形态：
 * - 服务端把非 ASCII 引用清洗成下划线（name 是韩文、ref 是 `_____________Neutral.webp`）；
 * - 立绘资源名带数字后缀（`anis_casual_surprised3`）而标记键不带；
 * - 同一个键多次解析必须稳定（不闪烁）。
 */
class CardAssetIndexTest {

    private val base = "http://192.168.5.5:8890"

    @Test
    fun `resolves korean name case-insensitively and via ref`() {
        val url = "/api/v1/cards/c1/assets/_____________Neutral.webp"
        val index = CardAssetResolver.index(mapOf("하세가와 아키라_평상복_Neutral" to url))

        // 标记里是小写 neutral，资源名是 Neutral
        assertEquals("$base$url", CardAssetResolver.resolveIndexed(index, "하세가와 아키라_평상복_neutral", base))
        // 标记里直接写 ref（带扩展名 / 不带扩展名）
        assertEquals("$base$url", CardAssetResolver.resolveIndexed(index, "_____________Neutral.webp", base))
        assertEquals("$base$url", CardAssetResolver.resolveIndexed(index, "_____________Neutral", base))
    }

    @Test
    fun `resolves numeric suffix variants deterministically`() {
        val index = CardAssetResolver.index(
            mapOf(
                "anis_casual_surprised3" to "/a/surprised3.webp",
                "anis_casual_playful_wink2" to "/a/wink2.webp",
            ),
        )
        assertEquals("$base/a/surprised3.webp", CardAssetResolver.resolveIndexed(index, "anis_casual_surprised", base))
        assertEquals("$base/a/wink2.webp", CardAssetResolver.resolveIndexed(index, "anis_casual_playful_wink", base))
        repeat(5) {
            assertEquals("$base/a/surprised3.webp", CardAssetResolver.resolveIndexed(index, "anis_casual_surprised", base))
        }
    }

    @Test
    fun `resolves across unicode normalization forms`() {
        val name = "하세가와_Neutral"
        val nfd = Normalizer.normalize(name, Normalizer.Form.NFD)
        val nfc = Normalizer.normalize(name, Normalizer.Form.NFC)
        // 资源表里是 NFD，标记里是 NFC（或反过来）都要命中
        val index = CardAssetResolver.index(mapOf(nfd to "/nfd.webp"))
        assertEquals("$base/nfd.webp", CardAssetResolver.resolveIndexed(index, nfc, base))

        val index2 = CardAssetResolver.index(mapOf(nfc to "/nfc.webp"))
        assertEquals("$base/nfc.webp", CardAssetResolver.resolveIndexed(index2, nfd, base))
    }

    @Test
    fun `empty index stays empty and never invents urls`() {
        val index = CardAssetResolver.index(emptyMap())
        assertTrue(index.isEmpty)
        assertNull(CardAssetResolver.resolveIndexed(index, "Soraka_smile", base))
        assertNull(CardAssetResolver.resolveIndexed(index, "", base))
    }

    @Test
    fun `absolute urls, paths and data uris pass through`() {
        val index = CardAssetResolver.index(mapOf("x" to "/y.webp"))
        assertEquals("https://cdn.example.com/a.png", CardAssetResolver.resolveIndexed(index, "https://cdn.example.com/a.png", base))
        assertEquals("data:image/png;base64,AAA", CardAssetResolver.resolveIndexed(index, "data:image/png;base64,AAA", base))
        assertEquals("$base/api/v1/cards/c1/assets/a%20b.webp", CardAssetResolver.resolveIndexed(index, "/api/v1/cards/c1/assets/a%20b.webp", base))
    }

    @Test
    fun `does not fuzzy match unrelated keys`() {
        val index = CardAssetResolver.index(mapOf("dressed_happy1.png" to "/h1.png"))
        assertNull(CardAssetResolver.resolveIndexed(index, "dressed_sad", base))
        assertNull(CardAssetResolver.resolveIndexed(index, "happy", base))
        assertNull(CardAssetResolver.resolveIndexed(index, "dressed_happy2", base))
    }
}
