package dev.tavernagent.mobile.playback

import android.content.Context
import android.speech.tts.TextToSpeech
import java.util.Locale

/**
 * 本地朗读（Android 系统 TTS）。后端 TTS（/api/v1/tts）作为可选项在设置中启用，
 * 这里提供默认可用的离线路径，避免"没配置后端就不能朗读"。
 */
object TtsEngine {

    private var tts: TextToSpeech? = null
    private var ready = false
    private var pending: String? = null
    private var rate: Float = 1.0f
    private var pitch: Float = 1.0f

    fun speak(context: Context, text: String) {
        val body = text.trim()
        if (body.isEmpty()) return
        val engine = ensure(context)
        if (ready) {
            engine.speak(body, TextToSpeech.QUEUE_FLUSH, null, "tavern-${System.currentTimeMillis()}")
        } else {
            pending = body
        }
    }

    fun stop() {
        tts?.stop()
        pending = null
    }

    fun configure(rate: Float, pitch: Float) {
        this.rate = rate
        this.pitch = pitch
        tts?.setSpeechRate(rate)
        tts?.setPitch(pitch)
    }

    private fun ensure(context: Context): TextToSpeech {
        tts?.let { return it }
        val created = TextToSpeech(context.applicationContext) { status ->
            ready = status == TextToSpeech.SUCCESS
            if (ready) {
                tts?.language = Locale.CHINESE
                tts?.setSpeechRate(rate)
                tts?.setPitch(pitch)
                pending?.let {
                    tts?.speak(it, TextToSpeech.QUEUE_FLUSH, null, "tavern-pending")
                    pending = null
                }
            }
        }
        tts = created
        return created
    }
}

fun readAloud(context: Context, text: String) = TtsEngine.speak(context, text)
