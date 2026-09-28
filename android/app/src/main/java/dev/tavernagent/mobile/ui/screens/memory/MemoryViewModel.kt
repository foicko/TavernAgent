package dev.tavernagent.mobile.ui.screens.memory

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import dev.tavernagent.mobile.TavernApp
import dev.tavernagent.mobile.data.remote.ApiException
import dev.tavernagent.mobile.data.remote.dto.MemoryPageDto
import dev.tavernagent.mobile.data.remote.dto.SessionViewDto
import dev.tavernagent.mobile.data.repo.MemoryRepository
import dev.tavernagent.mobile.data.repo.Keys
import dev.tavernagent.mobile.data.repo.StoryRepository
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * 记忆库：分页、筛选与写操作。
 * 写操作是分支级 copy-on-write，必须携带当前分支头与版本（乐观并发）。
 */
class MemoryViewModel(
    private val memories: MemoryRepository,
    private val story: StoryRepository,
) : ViewModel() {

    data class State(
        val loading: Boolean = true,
        val page: MemoryPageDto? = null,
        val view: SessionViewDto? = null,
        val search: String = "",
        val kind: String = "",
        val error: String? = null,
        val message: String? = null,
        val busy: Boolean = false,
        val loadingMore: Boolean = false,
        val editing: MemoryRow? = null,
    )

    data class MemoryRow(
        val memoryId: String,
        val content: String,
        val kind: String,
        val pinned: Boolean,
        val hidden: Boolean,
        val effective: Boolean,
        val protected: Boolean,
        val confidence: Double,
        val createdTurn: Int,
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    private var searchJob: Job? = null
    private var sessionId: String = ""
    private var branchId: String = ""
    // 写操作幂等键按"操作签名"复用：超时/结果未知后用户重试不会建出重复 CoW 修订。
    private val patchKeys = mutableMapOf<String, String>()
    // 分页加载代次：搜索/筛选变更会让在途的旧页请求作废，避免旧结果覆盖新结果。
    private var loadGeneration = 0

    fun open(sessionId: String, branchId: String) {
        this.sessionId = sessionId
        this.branchId = branchId.takeIf { it.isNotBlank() && it != "_" }.orEmpty()
        load()
        viewModelScope.launch {
            runCatching { story.view(sessionId, this@MemoryViewModel.branchId.ifBlank { null }) }
                .onSuccess { view ->
                    if (this@MemoryViewModel.branchId.isBlank()) this@MemoryViewModel.branchId = view.branch.branchId
                    _state.update { it.copy(view = view) }
                }
        }
    }

    fun refresh() = load()

    private fun load(cursor: String? = null) {
        if (cursor == null) loadGeneration++
        val generation = loadGeneration
        viewModelScope.launch {
            if (cursor == null) {
                _state.update { it.copy(loading = it.page == null, error = null) }
            } else {
                _state.update { it.copy(loadingMore = true) }
            }
            runCatching {
                memories.page(
                    sessionId = sessionId,
                    branchId = branchId,
                    nodeId = _state.value.view?.viewNodeId,
                    search = _state.value.search.takeIf { it.isNotBlank() },
                    kind = _state.value.kind.takeIf { it.isNotBlank() },
                    limit = 40,
                    cursor = cursor,
                )
            }.onSuccess { page ->
                if (generation != loadGeneration) return@onSuccess
                _state.update { current ->
                    val merged = if (cursor == null) page else page.copy(
                        memories = current.page?.memories.orEmpty() + page.memories,
                    )
                    current.copy(loading = false, loadingMore = false, page = merged, error = null)
                }
            }.onFailure { error ->
                if (generation != loadGeneration) return@onFailure
                _state.update { it.copy(loading = false, loadingMore = false, error = error.message ?: "加载记忆失败") }
            }
        }
    }

    fun loadMore() {
        val cursor = _state.value.page?.nextCursor ?: return
        if (cursor.isBlank()) return
        load(cursor)
    }

    fun onSearchChange(value: String) {
        _state.update { it.copy(search = value) }
        searchJob?.cancel()
        searchJob = viewModelScope.launch {
            delay(350)
            load()
        }
    }

    fun setKind(kind: String) {
        _state.update { it.copy(kind = kind) }
        load()
    }

    fun dismissEditor() = _state.update { it.copy(editing = null) }

    fun openEditor(row: MemoryRow) = _state.update { it.copy(editing = row) }

    fun togglePin(row: MemoryRow) = patch(row.memoryId, pinned = !row.pinned)

    fun toggleHidden(row: MemoryRow) = patch(row.memoryId, hidden = !row.hidden)

    fun revise(row: MemoryRow, content: String) {
        patch(row.memoryId, content = content)
        dismissEditor()
    }

    fun organize() {
        val view = _state.value.view ?: return
        viewModelScope.launch {
            _state.update { it.copy(busy = true) }
            runCatching { memories.organize(sessionId, branchId, view.characterId) }
                .onSuccess { plan ->
                    _state.update {
                        it.copy(
                            busy = false,
                            message = "整理完成：处理 ${plan.operations.size} 条，跳过 ${plan.skipped.size} 条",
                        )
                    }
                    load()
                }
                .onFailure { error -> _state.update { it.copy(busy = false, error = error.message ?: "整理失败") } }
        }
    }

    private fun patch(
        memoryId: String,
        content: String? = null,
        pinned: Boolean? = null,
        hidden: Boolean? = null,
    ) {
        val view = _state.value.view ?: return
        val signature = listOf(memoryId, content, pinned, hidden).joinToString("|")
        val key = patchKeys.getOrPut(signature) { Keys.new("mp") }
        viewModelScope.launch {
            _state.update { it.copy(busy = true, error = null) }
            runCatching {
                memories.patch(
                    sessionId = sessionId,
                    branchId = branchId,
                    memoryId = memoryId,
                    characterId = view.characterId,
                    expectedHeadId = view.branch.headNodeId,
                    expectedVersion = view.branch.version,
                    idempotencyKey = key,
                    content = content,
                    pinned = pinned,
                    hidden = hidden,
                )
            }.onSuccess {
                patchKeys.remove(signature)
                _state.update { current -> current.copy(busy = false, message = "已更新记忆") }
                load()
            }.onFailure { error ->
                // 4xx 明确拒绝：该键作废，下次重试用新键；其余（超时/网络）保留键以便安全重放。
                if (error is ApiException && error.status in 400..499) patchKeys.remove(signature)
                _state.update { it.copy(busy = false, error = error.message ?: "更新失败") }
            }
        }
    }

    fun clearMessage() = _state.update { it.copy(message = null) }

    companion object {
        fun factory() = viewModelFactory {
            initializer {
                MemoryViewModel(
                    TavernApp.instance.container.memoryRepository,
                    TavernApp.instance.container.storyRepository,
                )
            }
        }
    }
}
