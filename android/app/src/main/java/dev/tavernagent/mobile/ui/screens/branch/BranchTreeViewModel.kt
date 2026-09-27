package dev.tavernagent.mobile.ui.screens.branch

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import dev.tavernagent.mobile.TavernApp
import dev.tavernagent.mobile.data.remote.dto.GraphViewDto
import dev.tavernagent.mobile.data.remote.dto.PlotNodeDto
import dev.tavernagent.mobile.data.remote.dto.SessionViewDto
import dev.tavernagent.mobile.data.repo.StoryRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

class BranchTreeViewModel(private val repository: StoryRepository) : ViewModel() {

    data class State(
        val loading: Boolean = true,
        val view: SessionViewDto? = null,
        val graph: GraphViewDto? = null,
        val graphLoading: Boolean = false,
        val error: String? = null,
        val busy: Boolean = false,
        val message: String? = null,
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    fun load(sessionId: String, branchId: String?) {
        viewModelScope.launch {
            _state.update { it.copy(loading = true, error = null) }
            runCatching { repository.view(sessionId, branchId.takeIf { !it.isNullOrBlank() && it != "_" }) }
                .onSuccess { view ->
                    _state.update { it.copy(loading = false, view = view) }
                    loadGraph(view.sessionId, view.viewNodeId.ifBlank { view.branch.headNodeId })
                }
                .onFailure { error ->
                    _state.update { it.copy(loading = false, error = error.message ?: "加载失败") }
                }
        }
    }

    fun loadGraph(sessionId: String, nodeId: String) {
        viewModelScope.launch {
            _state.update { it.copy(graphLoading = true) }
            runCatching { repository.graph(sessionId, nodeId, null) }
                .onSuccess { graph -> _state.update { it.copy(graphLoading = false, graph = graph) } }
                .onFailure { _state.update { it.copy(graphLoading = false) } }
        }
    }

    fun fork(nodeId: String, onForked: (String) -> Unit) {
        val view = _state.value.view ?: return
        viewModelScope.launch {
            _state.update { it.copy(busy = true, error = null) }
            runCatching {
                repository.fork(
                    sessionId = view.sessionId,
                    fromNodeId = nodeId,
                    characterId = view.characterId,
                    name = "平行分支_${System.currentTimeMillis() % 10000}",
                )
            }.onSuccess { branch ->
                _state.update { it.copy(busy = false, message = "已开辟新分支「${branch.name}」") }
                onForked(branch.branchId)
            }.onFailure { error ->
                _state.update { it.copy(busy = false, error = error.message ?: "开辟分支失败") }
            }
        }
    }

    fun describe(node: PlotNodeDto): String = when (node.kind) {
        "root" -> "命运起点"
        "memory_change" -> "记忆整理 / 修订"
        "director_event" -> "导演安排 / 进度调整"
        "configuration_change" -> "配置变更"
        else -> "回合 #${node.turnNumber}"
    }

    fun clearMessage() = _state.update { it.copy(message = null) }

    companion object {
        fun factory() = viewModelFactory {
            initializer {
                BranchTreeViewModel(TavernApp.instance.container.storyRepository)
            }
        }
    }
}
