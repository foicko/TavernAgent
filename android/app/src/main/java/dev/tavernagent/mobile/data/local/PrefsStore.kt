package dev.tavernagent.mobile.data.local

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.floatPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

private val Context.userDataStore: DataStore<Preferences> by preferencesDataStore(name = "tavern_prefs")

enum class ThemeMode { SYSTEM, LIGHT, DARK, VIOLET }

/** 本地界面偏好与“上次读到哪儿”。 */
data class UserPrefs(
    val themeMode: ThemeMode = ThemeMode.SYSTEM,
    val lastSessionId: String = "",
    val lastBranchId: String = "",
    val lastNodeId: String = "",
    val lastSessionTitle: String = "",
    val ttsRate: Float = 1.0f,
    val ttsPitch: Float = 1.0f,
    val ttsVoice: String = "",
    val autoRead: Boolean = false,
    val streamThinking: Boolean = true,
)

class PrefsStore(private val context: Context) {

    val prefs: Flow<UserPrefs> = context.userDataStore.data.map { p ->
        UserPrefs(
            themeMode = runCatching { ThemeMode.valueOf(p[KEY_THEME].orEmpty()) }.getOrDefault(ThemeMode.SYSTEM),
            lastSessionId = p[KEY_LAST_SESSION].orEmpty(),
            lastBranchId = p[KEY_LAST_BRANCH].orEmpty(),
            lastNodeId = p[KEY_LAST_NODE].orEmpty(),
            lastSessionTitle = p[KEY_LAST_TITLE].orEmpty(),
            ttsRate = p[KEY_TTS_RATE] ?: 1.0f,
            ttsPitch = p[KEY_TTS_PITCH] ?: 1.0f,
            ttsVoice = p[KEY_TTS_VOICE].orEmpty(),
            autoRead = p[KEY_AUTO_READ] ?: false,
            streamThinking = p[KEY_STREAM_THINKING] ?: true,
        )
    }

    suspend fun setThemeMode(mode: ThemeMode) = context.userDataStore.edit { it[KEY_THEME] = mode.name }

    suspend fun setLastLocation(sessionId: String, branchId: String, nodeId: String, title: String = "") {
        context.userDataStore.edit { p ->
            p[KEY_LAST_SESSION] = sessionId
            p[KEY_LAST_BRANCH] = branchId
            p[KEY_LAST_NODE] = nodeId
            if (title.isNotBlank()) p[KEY_LAST_TITLE] = title
        }
    }

    suspend fun setTts(rate: Float, pitch: Float, voice: String, autoRead: Boolean) {
        context.userDataStore.edit { p ->
            p[KEY_TTS_RATE] = rate
            p[KEY_TTS_PITCH] = pitch
            p[KEY_TTS_VOICE] = voice
            p[KEY_AUTO_READ] = autoRead
        }
    }

    suspend fun setStreamThinking(enabled: Boolean) =
        context.userDataStore.edit { it[KEY_STREAM_THINKING] = enabled }

    private companion object {
        val KEY_THEME = stringPreferencesKey("theme_mode")
        val KEY_LAST_SESSION = stringPreferencesKey("last_session_id")
        val KEY_LAST_BRANCH = stringPreferencesKey("last_branch_id")
        val KEY_LAST_NODE = stringPreferencesKey("last_node_id")
        val KEY_LAST_TITLE = stringPreferencesKey("last_session_title")
        val KEY_TTS_RATE = floatPreferencesKey("tts_rate")
        val KEY_TTS_PITCH = floatPreferencesKey("tts_pitch")
        val KEY_TTS_VOICE = stringPreferencesKey("tts_voice")
        val KEY_AUTO_READ = booleanPreferencesKey("auto_read")
        val KEY_STREAM_THINKING = booleanPreferencesKey("stream_thinking")
    }
}
