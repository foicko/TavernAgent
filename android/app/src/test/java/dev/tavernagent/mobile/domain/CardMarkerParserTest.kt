package dev.tavernagent.mobile.domain

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class CardMarkerParserTest {

    @Test
    fun parsesPlainTexts() {
        val segs = CardMarkerParser.parse("Hello, world!") { null }
        assertEquals(1, segs.size)
        assertEquals(ContentSegment.Text("Hello, world!"), segs[0])
    }

    @Test
    fun parsesThoughtTags() {
        val input = "Before <thought>Thinking deeply...</thought> After"
        val segs = CardMarkerParser.parse(input) { null }
        assertEquals(3, segs.size)
        assertEquals(ContentSegment.Text("Before "), segs[0])
        assertEquals(ContentSegment.Thought("Thinking deeply..."), segs[1])
        assertEquals(ContentSegment.Text(" After"), segs[2])
    }

    @Test
    fun parsesImageTagsAndMarkdown() {
        val input = """Look at this <img cmd="sword.png"> and [IMG:shield] and ![Banner](https://img.com/banner.jpg)"""
        val segs = CardMarkerParser.parse(input) { key ->
            when (key) {
                "sword.png" -> "https://server.local/assets/sword.png"
                "shield" -> "https://server.local/assets/shield.png"
                else -> null
            }
        }
        assertEquals(6, segs.size)
        assertEquals(ContentSegment.Text("Look at this "), segs[0])
        assertEquals(ContentSegment.Image("https://server.local/assets/sword.png", "sword.png", "sword.png"), segs[1])
        assertEquals(ContentSegment.Text(" and "), segs[2])
        assertEquals(ContentSegment.Image("https://server.local/assets/shield.png", "shield", "shield"), segs[3])
        assertEquals(ContentSegment.Text(" and "), segs[4])
        assertEquals(ContentSegment.Image("https://img.com/banner.jpg", "Banner", "https://img.com/banner.jpg"), segs[5])
    }

    @Test
    fun parsesHudSceneAndStripsVar() {
        val input = "Intro [HUD:HP=100|MP=50] [SCENE:酒馆大堂] [VAR:gold=20] Outro"
        val segs = CardMarkerParser.parse(input) { null }
        assertEquals(5, segs.size)
        assertEquals(ContentSegment.Text("Intro "), segs[0])
        val hud = segs[1] as ContentSegment.Hud
        assertEquals("100", hud.values["HP"])
        assertEquals("50", hud.values["MP"])
        assertEquals(ContentSegment.Text(" "), segs[2])
        val scene = segs[3] as ContentSegment.Scene
        assertEquals("酒馆大堂", scene.title)
        assertEquals(ContentSegment.Text(" Outro"), segs[4])
    }

    @Test
    fun respectsEscaping() {
        val input = "Literal [[IMG:fake] and [[HUD:none]"
        val segs = CardMarkerParser.parse(input) { null }
        assertEquals(1, segs.size)
        assertEquals(ContentSegment.Text("Literal [IMG:fake] and [HUD:none]"), segs[0])
    }
}
