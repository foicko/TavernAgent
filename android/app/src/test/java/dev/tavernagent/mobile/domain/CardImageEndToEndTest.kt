package dev.tavernagent.mobile.domain

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * 用线上真实载荷做回归：正文里的标记 + 卡预览的 assets 表 → 图片 URL。
 *
 * 数据取自 TavernAgent 服务端的真实响应（Soraka charx 卡）：
 * - block.text: `…\n\n<img="Soraka_smile">`
 * - assets: name=Soraka_smile → /api/v1/cards/86158a4138ce0ca1/assets/Soraka_smile.webp
 */
class CardImageEndToEndTest {

    private val base = "http://192.168.5.5:8890"

    private val sorakaAssets = mapOf(
        "Soraka_smile" to "/api/v1/cards/86158a4138ce0ca1/assets/Soraka_smile.webp",
        "Soraka_curious" to "/api/v1/cards/86158a4138ce0ca1/assets/Soraka_curious.webp",
        "iconx" to "/api/v1/cards/86158a4138ce0ca1/assets/iconx.png",
    )

    @Test
    fun `real marker text resolves to real asset url`() {
        val text = "她把杯子收好，重新叠放在架子上。\n\n<img=\"Soraka_smile\">"
        val resolve: (String) -> String? = { key -> CardAssetResolver.resolve(sorakaAssets, key, base) }
        val segments = CardMarkerParser.parse(text, resolve)

        val images = segments.filterIsInstance<ContentSegment.Image>()
        assertEquals(1, images.size)
        assertEquals("$base/api/v1/cards/86158a4138ce0ca1/assets/Soraka_smile.webp", images.first().url)

        val plain = segments.filterIsInstance<ContentSegment.Text>().joinToString("") { it.content }
        assertTrue(plain.contains("她把杯子收好"))
        assertTrue("标记不应出现在正文里", !plain.contains("<img"))
    }

    @Test
    fun `multiple markers keep order and text between them`() {
        val text = "先看她一眼。<img=\"Soraka_smile\">她换了表情。<img=\"Soraka_curious\">"
        val resolve: (String) -> String? = { key -> CardAssetResolver.resolve(sorakaAssets, key, base) }
        val segments = CardMarkerParser.parse(text, resolve)

        val kinds = segments.map { it::class.simpleName }
        assertEquals(listOf("Text", "Image", "Text", "Image"), kinds)
        val urls = segments.filterIsInstance<ContentSegment.Image>().map { it.url }
        assertEquals(
            listOf(
                "$base/api/v1/cards/86158a4138ce0ca1/assets/Soraka_smile.webp",
                "$base/api/v1/cards/86158a4138ce0ca1/assets/Soraka_curious.webp",
            ),
            urls,
        )
    }

    @Test
    fun `unmapped key is not invented`() {
        val resolve: (String) -> String? = { key -> CardAssetResolver.resolve(sorakaAssets, key, base) }
        assertEquals(null, resolve("Soraka_embarrassed"))
    }

    @Test
    fun `unmapped marker becomes a missing-asset image segment`() {
        val text = "她犹豫了一下。<img=\"Soraka_embarrassed\">"
        val resolve: (String) -> String? = { key -> CardAssetResolver.resolve(sorakaAssets, key, base) }
        val segments = CardMarkerParser.parse(text, resolve)

        val images = segments.filterIsInstance<ContentSegment.Image>()
        assertEquals(1, images.size)
        assertEquals("", images.first().url)
        assertEquals("Soraka_embarrassed", images.first().rawKey)

        val plain = segments.filterIsInstance<ContentSegment.Text>().joinToString("") { it.content }
        assertTrue("裸标记不应出现在正文里", !plain.contains("<img"))
    }
}
