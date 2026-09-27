package dev.tavernagent.mobile.ui.screens.characters

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import dev.tavernagent.mobile.TavernApp
import dev.tavernagent.mobile.data.remote.ApiJson
import dev.tavernagent.mobile.data.remote.dto.SessionViewDto
import dev.tavernagent.mobile.data.remote.dto.WorldStateDto
import dev.tavernagent.mobile.data.repo.StoryRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

class CharactersViewModel(private val repository: StoryRepository) : ViewModel() {

    data class State(
        val loading: Boolean = true,
        val view: SessionViewDto? = null,
        val world: WorldStateDto? = null,
        val error: String? = null,
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    fun load(sessionId: String, branchId: String?) {
        viewModelScope.launch {
            _state.update { it.copy(loading = true, error = null) }
            runCatching {
                repository.view(sessionId, branchId?.takeIf { it.isNotBlank() && it != "_" })
            }.onSuccess { view ->
                val world = view.state?.let { runCatching { ApiJson.from<WorldStateDto>(it) }.getOrNull() }
                _state.update { it.copy(loading = false, view = view, world = world) }
            }.onFailure { error ->
                _state.update { it.copy(loading = false, error = error.message ?: "加载失败") }
            }
        }
    }

    companion object {
        fun factory() = viewModelFactory {
            initializer { CharactersViewModel(TavernApp.instance.container.storyRepository) }
        }
    }
}
