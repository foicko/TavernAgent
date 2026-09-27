package dev.tavernagent.mobile.ui.screens.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import dev.tavernagent.mobile.TavernApp
import dev.tavernagent.mobile.data.local.PrefsStore
import dev.tavernagent.mobile.data.local.ThemeMode
import dev.tavernagent.mobile.data.remote.ApiProvider
import dev.tavernagent.mobile.data.repo.ConnectionRepository
import dev.tavernagent.mobile.data.repo.MediaRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

class SettingsViewModel(
    private val connection: ConnectionRepository,
    private val prefs: PrefsStore,
    private val media: MediaRepository,
    private val apiProvider: ApiProvider,
) : ViewModel() {

    data class State(
        val loading: Boolean = true,
        val baseUrl: String = "",
        val themeMode: ThemeMode = ThemeMode.SYSTEM,
        val ttsRate: Float = 1.0f,
        val ttsPitch: Float = 1.0f,
        val autoRead: Boolean = false,
        val backendTtsEnabled: Boolean = false,
        val message: String? = null,
        val voices: List<dev.tavernagent.mobile.data.remote.dto.VoiceDto> = emptyList(),
        val selectedVoice: String = "",
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    init {
        // 连接状态实时跟随凭据。原实现把 baseUrl 的写入放在了永不返回的 collect 之后，
        // 导致设置页始终显示「未连接」——这里拆成独立协程修正。
        viewModelScope.launch {
            connection.credentials.collect { creds ->
                _state.update { it.copy(loading = false, baseUrl = creds.baseUrl) }
            }
        }
        viewModelScope.launch {
            prefs.prefs.collect { userPrefs ->
                _state.update {
                    it.copy(
                        themeMode = userPrefs.themeMode,
                        ttsRate = userPrefs.ttsRate,
                        ttsPitch = userPrefs.ttsPitch,
                        autoRead = userPrefs.autoRead,
                    )
                }
            }
        }
        viewModelScope.launch {
            runCatching { media.voices() }
                .onSuccess { voices -> _state.update { it.copy(voices = voices, selectedVoice = it.selectedVoice.ifBlank { voices.firstOrNull()?.id.orEmpty() }) } }
        }
    }

    fun setTheme(mode: ThemeMode) {
        viewModelScope.launch { prefs.setThemeMode(mode) }
    }

    fun setTts(rate: Float, pitch: Float, autoRead: Boolean) {
        viewModelScope.launch { prefs.setTts(rate, pitch, _state.value.selectedVoice, autoRead) }
    }

    fun setAutoRead(enabled: Boolean) {
        _state.update { it.copy(autoRead = enabled) }
        viewModelScope.launch { prefs.setTts(_state.value.ttsRate, _state.value.ttsPitch, _state.value.selectedVoice, enabled) }
    }

    fun selectVoice(voiceId: String) {
        _state.update { it.copy(selectedVoice = voiceId) }
        viewModelScope.launch { prefs.setTts(_state.value.ttsRate, _state.value.ttsPitch, voiceId, _state.value.autoRead) }
    }

    fun testTts(speak: (String) -> Unit) {
        speak("这是一段朗读测试。酒馆的灯还亮着，故事继续。")
    }

    /** 后端 TTS 连通性测试：成功时回调音频字节交给播放器。 */
    fun testBackendTts(onAudio: (ByteArray) -> Unit) {
        viewModelScope.launch {
            runCatching {
                media.synthesize(
                    voice = _state.value.selectedVoice.ifBlank { "zh-CN-XiaoxiaoNeural" },
                    text = "后端语音合成连通性测试。",
                    speed = _state.value.ttsRate.toDouble(),
                )
            }.onSuccess { bytes ->
                _state.update { it.copy(message = "后端语音合成成功（${bytes.size / 1024} KB）") }
                onAudio(bytes)
            }.onFailure { error ->
                _state.update { it.copy(message = "后端语音合成失败：${error.message}") }
            }
        }
    }

    fun generateTestImage(prompt: String, onImage: (String) -> Unit) {
        viewModelScope.launch {
            runCatching { media.generateImage(prompt = prompt, size = "1024x1024") }
                .onSuccess { image -> onImage(image.url) }
                .onFailure { error -> _state.update { it.copy(message = "生成插画失败：${error.message}") } }
        }
    }

    fun disconnect() {
        viewModelScope.launch { connection.forget() }
    }

    fun clearMessage() = _state.update { it.copy(message = null) }

    companion object {
        fun factory() = viewModelFactory {
            initializer {
                SettingsViewModel(
                    TavernApp.instance.container.connectionRepository,
                    TavernApp.instance.container.prefsStore,
                    TavernApp.instance.container.mediaRepository,
                    TavernApp.instance.container.apiProvider,
                )
            }
        }
    }
}
