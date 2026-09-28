package dev.tavernagent.mobile.ui.screens.director

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import dev.tavernagent.mobile.TavernApp
import dev.tavernagent.mobile.data.remote.dto.DirectorBeatDto
import dev.tavernagent.mobile.data.remote.dto.DirectorPlanDto
import dev.tavernagent.mobile.data.remote.dto.DirectorRequestDto
import dev.tavernagent.mobile.data.remote.dto.DirectorViewDto
import dev.tavernagent.mobile.data.remote.dto.SessionViewDto
import dev.tavernagent.mobile.data.repo.DirectorRepository
import dev.tavernagent.mobile.data.repo.Keys
import dev.tavernagent.mobile.data.repo.StoryRepository
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.mapNotNull
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull

/** 导演讨论最长等待（SSE）；超时给出显式提示而不是静默退出。 */
private const val DIRECTOR_STREAM_TIMEOUT_MS = 5 * 60 * 1000L

/** 导演模式：大纲草稿编辑、讨论（异步终态 + 轮询）、三阶段命令。 */
class DirectorViewModel(
    private val director: DirectorRepository,
    private val story: StoryRepository,
) : ViewModel() {

    data class State(
        val loading: Boolean = true,
        val view: SessionViewDto? = null,
        val directorView: DirectorViewDto? = null,
        val error: String? = null,
        val busy: Boolean = false,
        val message: String? = null,
        val discussion: String = "",
        val activeRequest: DirectorRequestDto? = null,
        val polling: Boolean = false,
        val draftTitle: String = "",
        val draftGuidance: String = "",
        val draftBeats: List<DirectorBeatDto> = emptyList(),
        val draftDirty: Boolean = false,
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    private var sessionId: String = ""
    private var branchId: String = ""
    private var pollJob: Job? = null

    fun open(sessionId: String, branchId: String) {
        this.sessionId = sessionId
        this.branchId = branchId.takeIf { it.isNotBlank() && it != "_" }.orEmpty()
        viewModelScope.launch {
            _state.update { it.copy(loading = true, error = null) }
            val view = runCatching { story.view(sessionId, this@DirectorViewModel.branchId.ifBlank { null }) }.getOrNull()
            if (view != null && this@DirectorViewModel.branchId.isBlank()) this@DirectorViewModel.branchId = view.branch.branchId
            runCatching { director.view(sessionId, this@DirectorViewModel.branchId, view?.viewNodeId) }
                .onSuccess { directorView ->
                    val plan = directorView.draft?.plan ?: directorView.state?.plan ?: DirectorPlanDto()
                    _state.update {
                        it.copy(
                            loading = false,
                            view = view,
                            directorView = directorView,
                            error = null,
                            draftTitle = plan.title,
                            draftGuidance = plan.guidance,
                            draftBeats = plan.beats,
                            draftDirty = false,
                        )
                    }
                    directorView.requests.firstOrNull { it.status == "generating" }?.let { pollRequest(it.requestId) }
                }
                .onFailure { error ->
                    _state.update { it.copy(loading = false, error = error.message ?: "导演服务不可用") }
                }
        }
    }

    fun refresh() = open(sessionId, branchId)

    fun editTitle(value: String) = _state.update { it.copy(draftTitle = value, draftDirty = true) }
    fun editGuidance(value: String) = _state.update { it.copy(draftGuidance = value, draftDirty = true) }
    fun editBeat(index: Int, beat: DirectorBeatDto) = _state.update {
        it.copy(draftBeats = it.draftBeats.toMutableList().also { beats -> beats[index] = beat }, draftDirty = true)
    }

    fun addBeat() = _state.update {
        it.copy(
            draftBeats = it.draftBeats + DirectorBeatDto(beatId = "", title = "新节拍", instruction = "", completionCriteria = ""),
            draftDirty = true,
        )
    }

    fun removeBeat(index: Int) = _state.update {
        it.copy(draftBeats = it.draftBeats.toMutableList().also { beats -> beats.removeAt(index) }, draftDirty = true)
    }

    fun saveDraft() {
        val snapshot = _state.value
        val characterId = snapshot.view?.characterId.orEmpty()
        val draft = snapshot.directorView?.draft
        viewModelScope.launch {
            _state.update { it.copy(busy = true, error = null) }
            runCatching {
                director.saveDraft(
                    sessionId = sessionId,
                    branchId = branchId,
                    characterId = characterId,
                    expectedDraftVersion = draft?.version ?: 0,
                    baseRevisionId = draft?.baseRevisionId ?: snapshot.directorView?.state?.plan?.revisionId.orEmpty(),
                    plan = DirectorPlanDto(
                        planId = draft?.plan?.planId,
                        revisionId = draft?.plan?.revisionId,
                        title = snapshot.draftTitle,
                        guidance = snapshot.draftGuidance,
                        beats = snapshot.draftBeats,
                    ),
                )
            }.onSuccess { saved ->
                _state.update {
                    it.copy(
                        busy = false,
                        message = "草稿已保存",
                        draftDirty = false,
                        directorView = it.directorView?.copy(draft = saved),
                    )
                }
            }.onFailure { error ->
                _state.update { it.copy(busy = false, error = error.message ?: "保存草稿失败") }
            }
        }
    }

    fun command(action: String, beatId: String? = null, replace: Boolean = true) {
        val snapshot = _state.value
        val view = snapshot.view ?: return
        viewModelScope.launch {
            _state.update { it.copy(busy = true, error = null) }
            runCatching {
                director.command(
                    sessionId = sessionId,
                    branchId = branchId,
                    characterId = view.characterId,
                    expectedHeadId = view.branch.headNodeId,
                    expectedVersion = view.branch.version,
                    action = action,
                    beatId = beatId,
                    draftVersion = snapshot.directorView?.draft?.version ?: 0,
                    replace = replace,
                )
            }.onSuccess { result ->
                _state.update { it.copy(busy = false, message = "已执行：$action") }
                val newView = runCatching { story.view(view.sessionId, branchId, null, 100) }.getOrNull()
                if (newView != null) _state.update { it.copy(view = newView) }
                result.state?.let { state ->
                    _state.update {
                        it.copy(
                            directorView = it.directorView?.copy(state = state),
                            draftBeats = state.plan.beats,
                            draftTitle = state.plan.title,
                            draftGuidance = state.plan.guidance,
                        )
                    }
                }
            }.onFailure { error ->
                _state.update { it.copy(busy = false, error = error.message ?: "命令执行失败") }
            }
        }
    }

    fun onDiscussionChange(value: String) = _state.update { it.copy(discussion = value) }

    fun discuss() {
        val snapshot = _state.value
        val view = snapshot.view ?: return
        val text = snapshot.discussion.trim()
        if (text.isEmpty()) return
        viewModelScope.launch {
            _state.update { it.copy(busy = true, error = null, discussion = "") }
            runCatching {
                director.discuss(
                    sessionId = sessionId,
                    branchId = branchId,
                    characterId = view.characterId,
                    expectedHeadId = view.branch.headNodeId,
                    expectedDraftVersion = snapshot.directorView?.draft?.version ?: 0,
                    text = text,
                )
            }.onSuccess { response ->
                _state.update { it.copy(busy = false) }
                pollRequest(response.requestId)
            }.onFailure { error ->
                _state.update { it.copy(busy = false, error = error.message ?: "发起讨论失败") }
            }
        }
    }

    private fun pollRequest(requestId: String) {
        pollJob?.cancel()
        pollJob = viewModelScope.launch {
            _state.update { it.copy(polling = true) }
            // 优先用导演 SSE：一条长连接替代原先 120 次 × 2s 的轮询。
            // 事件到达即拉一次请求状态（终态带回候选/错误），`first` 命中终态后自动取消上游。
            val streamed = runCatching {
                withTimeoutOrNull(DIRECTOR_STREAM_TIMEOUT_MS) {
                    director.events(requestId)
                        .mapNotNull { runCatching { director.request(requestId) }.getOrNull() }
                        .onEach { request -> _state.update { it.copy(activeRequest = request) } }
                        .first { it.status != "generating" }
                }
            }.getOrNull()
            val request = streamed ?: pollFallback(requestId)
            _state.update { it.copy(polling = false) }
            if (request == null) {
                _state.update { it.copy(error = "导演讨论超时，请稍后重试") }
            } else {
                refresh()
            }
        }
    }

    /** SSE 不可用/提前断开时的兜底轮询：少量尝试后退避，最终交由显式超时提示。 */
    private suspend fun pollFallback(requestId: String): DirectorRequestDto? {
        repeat(8) {
            val request = runCatching { director.request(requestId) }.getOrNull()
            if (request != null) {
                _state.update { it.copy(activeRequest = request) }
                if (request.status != "generating") return request
            }
            delay(2_000)
        }
        return null
    }

    fun applyCandidate() {
        val candidate = _state.value.activeRequest?.candidate ?: return
        _state.update {
            it.copy(
                draftTitle = candidate.title,
                draftGuidance = candidate.guidance,
                draftBeats = candidate.beats,
                draftDirty = true,
                message = "候选已填入草稿，保存后可启用",
            )
        }
    }

    fun cancelRequest() {
        val requestId = _state.value.activeRequest?.requestId ?: return
        viewModelScope.launch {
            runCatching { director.cancelRequest(requestId, _state.value.view?.characterId.orEmpty()) }
            pollJob?.cancel()
            _state.update { it.copy(polling = false) }
            refresh()
        }
    }

    fun stopPolling() {
        pollJob?.cancel()
        _state.update { it.copy(polling = false) }
    }

    fun clearMessage() = _state.update { it.copy(message = null) }

    companion object {
        fun factory() = viewModelFactory {
            initializer {
                DirectorViewModel(
                    TavernApp.instance.container.directorRepository,
                    TavernApp.instance.container.storyRepository,
                )
            }
        }
    }
}
