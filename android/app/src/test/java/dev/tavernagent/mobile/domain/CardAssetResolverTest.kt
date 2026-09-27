package dev.tavernagent.mobile.domain

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class CardAssetResolverTest {

    private val baseUrl = "http://192.168.1.100:8890"

    @Test
    fun resolvesExactMatch() {
        val assets = mapOf("Soraka_smile" to "/api/v1/cards/c1/assets/Soraka_smile.webp")
        val result = CardAssetResolver.resolve(assets, "Soraka_smile", baseUrl)
        assertEquals("http://192.168.1.100:8890/api/v1/cards/c1/assets/Soraka_smile.webp", result)
    }

    @Test
    fun resolvesCaseInsensitive() {
        val assets = mapOf("has_Neutral" to "/n.webp")
        val result = CardAssetResolver.resolve(assets, "has_neutral", baseUrl)
        assertEquals("http://192.168.1.100:8890/n.webp", result)
    }

    @Test
    fun resolvesStrippingImageExtensions() {
        val assets = mapOf(
            "robe.default.avif" to "/r.avif",
            "main" to "/m.png",
            "bg.png" to "/bg_real.png",
        )
        // 标记不带后缀，资源带后缀
        assertEquals("http://192.168.1.100:8890/r.avif", CardAssetResolver.resolve(assets, "robe.default", baseUrl))
        // 标记带后缀，资源不带后缀
        assertEquals("http://192.168.1.100:8890/m.png", CardAssetResolver.resolve(assets, "main.png", baseUrl))
        // 标记不带后缀，资源带不同后缀
        assertEquals("http://192.168.1.100:8890/bg_real.png", CardAssetResolver.resolve(assets, "bg", baseUrl))
    }

    @Test
    fun resolvesNumericSuffix() {
        val assets = mapOf(
            "dressed_happy1.png" to "/h1.png",
            "dressed_neutral2.webp" to "/n2.webp",
        )
        assertEquals("http://192.168.1.100:8890/h1.png", CardAssetResolver.resolve(assets, "dressed_happy", baseUrl))
        assertEquals("http://192.168.1.100:8890/n2.webp", CardAssetResolver.resolve(assets, "dressed_neutral", baseUrl))
    }

    @Test
    fun doesNotStripNonImageExtensions() {
        val assets = mapOf("nami.flustered" to "/n")
        assertNull(CardAssetResolver.resolve(assets, "nami", baseUrl))
    }

    @Test
    fun leavesHttpAndDataUrlsUntouched() {
        val httpUrl = "https://example.com/pic.png"
        val dataUrl = "data:image/jpeg;base64,12345"
        assertEquals(httpUrl, CardAssetResolver.resolve(emptyMap(), httpUrl, baseUrl))
        assertEquals(dataUrl, CardAssetResolver.resolve(emptyMap(), dataUrl, baseUrl))
    }
}
