package dev.tavernagent.mobile.ui.screens.archive

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import dev.tavernagent.mobile.TavernApp
import dev.tavernagent.mobile.data.remote.dto.SessionDto
import dev.tavernagent.mobile.data.repo.StoryRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** 故事归档：导出 / 导入整段故事（.tavernpack）。 */
class ArchiveViewModel(private val repository: StoryRepository) : ViewModel() {

    data class State(
        val loading: Boolean = true,
        val sessions: List<SessionDto> = emptyList(),
        val error: String? = null,
        val busy: Boolean = false,
        val message: String? = null,
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _state.update { it.copy(loading = it.sessions.isEmpty(), error = null) }
            runCatching { repository.sessions() }
                .onSuccess { sessions -> _state.update { it.copy(loading = false, sessions = sessions) } }
                .onFailure { error -> _state.update { it.copy(loading = false, error = error.message ?: "加载失败") } }
        }
    }

    /** 导出为字节（由界面写入用户选择的位置）。 */
    suspend fun export(sessionId: String): Result<ByteArray> {
        _state.update { it.copy(busy = true) }
        val result = runCatching { repository.exportSession(sessionId) }
        _state.update { it.copy(busy = false) }
        result.onSuccess { _state.update { current -> current.copy(message = "已导出 ${it.size / 1024} KB") } }
            .onFailure { error -> _state.update { current -> current.copy(error = error.message ?: "导出失败") } }
        return result
    }

    fun import(bytes: ByteArray) {
        viewModelScope.launch {
            _state.update { it.copy(busy = true, error = null) }
            runCatching { repository.importSession(bytes) }
                .onSuccess { imported ->
                    _state.update { it.copy(busy = false, message = "已导入《${imported.title}》") }
                    load()
                }
                .onFailure { error ->
                    _state.update { it.copy(busy = false, error = error.message ?: "导入失败") }
                }
        }
    }

    fun clearMessage() = _state.update { it.copy(message = null) }

    companion object {
        fun factory() = viewModelFactory {
            initializer { ArchiveViewModel(TavernApp.instance.container.storyRepository) }
        }
    }
}
