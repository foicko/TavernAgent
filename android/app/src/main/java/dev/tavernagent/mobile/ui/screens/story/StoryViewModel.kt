package dev.tavernagent.mobile.ui.screens.story

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import dev.tavernagent.mobile.TavernApp
import dev.tavernagent.mobile.data.remote.ApiException
import dev.tavernagent.mobile.data.remote.ApiJson
import dev.tavernagent.mobile.data.remote.api.TurnInputDto
import dev.tavernagent.mobile.data.remote.dto.ModelInstanceDto
import dev.tavernagent.mobile.data.remote.dto.OptionDto
import dev.tavernagent.mobile.data.remote.dto.SessionViewDto
import dev.tavernagent.mobile.data.remote.dto.SlotBindingDto
import dev.tavernagent.mobile.data.remote.dto.WorldStateDto
import dev.tavernagent.mobile.data.repo.Keys
import dev.tavernagent.mobile.data.repo.CardsRepository
import dev.tavernagent.mobile.data.repo.ModelsRepository
import dev.tavernagent.mobile.data.repo.StoryRepository
import dev.tavernagent.mobile.domain.DraftBlock
import dev.tavernagent.mobile.domain.StoryMessage
import dev.tavernagent.mobile.domain.StoryMessages
import dev.tavernagent.mobile.domain.TurnPhase
import dev.tavernagent.mobile.util.runCatchingCancellable
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull

/** 思考文本/时长缓存的容量上限（按最近使用保留）。 */
private const val THINKING_CACHE_LIMIT = 40

/** 玩家消息的乐观回显：发送成功后立刻显示，提交后由服务端节点接管。 */
data class PendingEcho(
    val kind: String = "text",
    val text: String = "",
    val note: String? = null,
)

data class StoryUiState(
    val sessionId: String = "",
    val branchId: String = "",
    val title: String = "",
    val baseUrl: String = "",
    val autoRead: Boolean = false,
    val loading: Boolean = true,
    val refreshing: Boolean = false,
    val view: SessionViewDto? = null,
    val worldState: WorldStateDto? = null,
    val messages: List<StoryMessage> = emptyList(),
    val draftBlocks: List<DraftBlock> = emptyList(),
    val thinking: String = "",
    val thinkingSeconds: Int? = null,
    val phase: TurnPhase = TurnPhase.IDLE,
    val currentTurnId: String? = null,
    val error: String? = null,
    val errorRetryable: Boolean = true,
    val inputText: String = "",
    val inputNote: String = "",
    val noteVisible: Boolean = false,
    val optionsMode: String = "auto",
    val viewingNodeId: String? = null,
    val historyLoading: Boolean = false,
    val busyAction: String? = null,
    val toast: String? = null,
    val characterName: String = "",
    val characterAvatarUrl: String? = null,
    val playerName: String = "玩家",
    val playerAvatarUrl: String? = null,
    val cardAssets: Map<String, String> = emptyMap(),
    val pendingEcho: PendingEcho? = null,
    val models: List<ModelInstanceDto> = emptyList(),
    val slots: List<SlotBindingDto> = emptyList(),
) {
    val capabilities get() = view?.capabilities
    val characterId: String get() = view?.characterId.orEmpty()
    val headNodeId: String get() = view?.branch?.headNodeId.orEmpty()
    val branchVersion: Long get() = view?.branch?.version ?: 0
    val viewingHistory: Boolean get() = !viewingNodeId.isNullOrBlank()
    val busy: Boolean get() = phase.isBusy || capabilities?.busy == true || busyAction != null
    val canSend: Boolean get() = !busy && !viewingHistory && (capabilities?.canSend ?: true)
    val lastTurnNodeId: String
        get() = view?.nodes?.lastOrNull { it.kind == "turn" }?.nodeId ?: headNodeId

    val activePrimarySlot: SlotBindingDto?
        get() = slots.firstOrNull { it.slot == "primary" && it.enabled }
            ?: slots.firstOrNull { it.slot == "primary" }

    val activePrimaryModel: ModelInstanceDto?
        get() {
            val modelId = activePrimarySlot?.modelId ?: activePrimarySlot?.resolvedModelId
            return models.firstOrNull { it.id == modelId }
        }

    val activePrimaryModelName: String
        get() = activePrimaryModel?.name?.takeIf { it.isNotBlank() }
            ?: activePrimaryModel?.model?.takeIf { it.isNotBlank() }
            ?: activePrimarySlot?.model?.takeIf { it.isNotBlank() }
            ?: "默认模型"

    val reasoningEffort: String
        get() = activePrimaryModel?.reasoningEffort.orEmpty()

    val resolvedCharacterName: String
        get() = characterName.ifBlank {
            worldState?.characters?.values?.firstOrNull { it.participant && it.characterId != "player" }?.name
                ?: worldState?.characters?.values?.firstOrNull { it.characterId != "player" }?.name
                ?: title.ifBlank { "角色" }
        }

    val resolvedPlayerName: String
        get() = playerName.ifBlank {
            worldState?.characters?.get("player")?.name ?: "玩家"
        }
}

/** 思考强度档位标签 */
fun effortLabel(effort: String?): String = when (effort) {
    "low" -> "低"
    "medium" -> "中"
    "high" -> "高"
    else -> "默认"
}

/**
 * 叙事阅读与推进（功能 3/4/5 的驱动）。
 *
 * 能力与可写性全部取自服务端 `capabilities`；本地只维护"瞬时相位"与草稿合并。
 * 同一幂等键在结果未知的重试中必须复用（与 Web 端一致）。
 */
class StoryViewModel(
    private val repository: StoryRepository,
    private val cardsRepository: CardsRepository = TavernApp.instance.container.cardsRepository,
    private val modelsRepository: ModelsRepository = TavernApp.instance.container.modelsRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(StoryUiState())
    val state: StateFlow<StoryUiState> = _state.asStateFlow()

    private var streamJob: Job? = null
    private var pendingKey: String? = null
    // 幂等载荷基线：重试必须复用同一 (key, input, expectedHeadId, expectedVersion)，
    // 否则 head/version 变动会让同键不同值 → 409 IDEMPOTENCY_CONFLICT（假失败）。
    private var pendingHeadId: String? = null
    private var pendingBranchVersion: Long? = null
    private var pendingInput: TurnInputDto? = null
    // 最近一次被服务端受理的输入：终态失败时用它重发一个新回合（而不是重放已终态的回合）。
    private var lastInput: TurnInputDto? = null
    private var assetsKey: String = ""
    // 思考文本/时长按 turnId 累积：长会话下必须限容（保留最近 N 个回合），否则无界增长。
    private val thinkingByTurn = object : LinkedHashMap<String, String>(16, 0.75f, true) {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<String, String>?): Boolean = size > THINKING_CACHE_LIMIT
    }
    private val thinkingSecondsByTurn = object : LinkedHashMap<String, Int>(16, 0.75f, true) {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<String, Int>?): Boolean = size > THINKING_CACHE_LIMIT
    }

    init {
        // 「自动朗读新回复」读本机偏好；聊天页在这个开关打开时，于提交后朗读最新一条。
        viewModelScope.launch {
            repository.lastLocation.collect { prefs ->
                _state.update { it.copy(autoRead = prefs.autoRead) }
            }
        }
        loadModels()
    }

    fun open(sessionId: String, branchArg: String?, viewNodeArg: String?) {
        _state.update {
            it.copy(
                sessionId = sessionId,
                loading = it.view?.sessionId != sessionId,
                viewingNodeId = viewNodeArg?.takeIf { value -> value.isNotBlank() && value != "_" },
            )
        }
        viewModelScope.launch {
            val base = runCatchingCancellable { repository.baseUrl() }.getOrDefault("")
            _state.update { it.copy(baseUrl = base) }
        }
        loadModels()
        reload(full = true, branchHint = branchArg?.takeIf { it.isNotBlank() && it != "_" })
    }

    fun refresh() {
        loadModels()
        reload(full = true, branchHint = null)
    }

    private fun reload(full: Boolean, branchHint: String? = null, before: String? = null) {
        viewModelScope.launch {
            _state.update { it.copy(refreshing = true, error = null, historyLoading = before != null) }
            val snapshot = _state.value
            runCatchingCancellable {
                repository.view(
                    sessionId = snapshot.sessionId,
                    branchId = branchHint ?: snapshot.branchId.takeIf { it.isNotBlank() },
                    viewNodeId = snapshot.viewingNodeId,
                    limit = 100,
                    before = before,
                )
            }.onSuccess { view ->
                // before != null 表示"加载更早"：服务端只返回更早的窗口，
                // 必须与当前（更新的）消息合并，否则最近回合会从屏幕消失。
                val previous = _state.value.messages
                applyView(view, keepViewingNode = before != null)
                if (before != null && previous.isNotEmpty()) {
                    _state.update { st ->
                        val olderIds = st.messages.map { it.nodeId }.toSet()
                        st.copy(messages = st.messages + previous.filter { it.nodeId !in olderIds })
                    }
                }
                repository.rememberLocation(view.sessionId, view.branch.branchId, view.branch.headNodeId, view.title)
                val activeTurn = view.branch.activeTurnId
                if (!activeTurn.isNullOrBlank() && streamJob?.isActive != true) {
                    startStream(activeTurn)
                }
            }.onFailure { error ->
                _state.update {
                    it.copy(
                        refreshing = false,
                        historyLoading = false,
                        loading = false,
                        error = error.message ?: "加载失败",
                        errorRetryable = true,
                    )
                }
            }
        }
    }

    /** 幂等键与其并发基线必须一起失效，避免"新键配旧基线"。 */
    private fun clearPendingKey() {
        pendingKey = null
        pendingHeadId = null
        pendingBranchVersion = null
    }

    private suspend fun applyView(view: SessionViewDto, keepViewingNode: Boolean = false) {
        // 解析可能上百个回合的 contentJson 与世界状态：放到 Default 线程，避免主线程卡顿/ANR。
        // 思考缓存先做快照，避免后台解析与主线程写入并发访问同一 map。
        val thinkingSnapshot = HashMap(thinkingByTurn)
        val secondsSnapshot = HashMap(thinkingSecondsByTurn)
        val (messages, worldState) = withContext(Dispatchers.Default) {
            StoryMessages.fromView(view, thinkingSnapshot, secondsSnapshot) to
                view.state?.let { runCatchingCancellable { ApiJson.from<WorldStateDto>(it) }.getOrNull() }
        }
        val charId = view.characterId
        val base = _state.value.baseUrl
        val playerAvatar = worldState?.characters?.get("player")?.avatar?.let { av ->
            when {
                av.startsWith("http") || av.startsWith("data:") -> av
                av.startsWith("/") -> if (base.isNotBlank()) "$base$av" else av
                else -> null
            }
        }

        // 已在本进程加载过的卡：同一帧种下资源表，避免「先按裸键 404、再重解析」的空窗。
        val cached = if (charId.isNotBlank() && base.isNotBlank()) cardsRepository.cachedAssets(charId) else null
        val cachedAvatar = cached?.let {
            resolveAvatar(
                it.avatar,
                dev.tavernagent.mobile.domain.CardAssetResolver.index(it.assets),
                base,
            )
        }

        _state.update {
            it.copy(
                branchId = view.branch.branchId,
                title = view.title,
                loading = false,
                refreshing = false,
                historyLoading = false,
                view = view,
                worldState = worldState,
                messages = messages,
                viewingNodeId = if (keepViewingNode) {
                    it.viewingNodeId
                } else {
                    // 由服务端返回的视图推导，而不是"加载即清空"：否则进入历史节点后
                    // 只读态立刻消失、canSend 变真，用户以为在最新处却其实在浏览历史。
                    view.viewNodeId.takeIf { id -> id.isNotBlank() && id != view.branch.headNodeId }
                },
                error = if (it.phase == TurnPhase.FAILED) it.error else null,
                playerAvatarUrl = playerAvatar ?: it.playerAvatarUrl,
                characterName = cached?.name?.takeIf { name -> name.isNotBlank() } ?: it.characterName,
                characterAvatarUrl = cachedAvatar ?: it.characterAvatarUrl,
                cardAssets = cached?.assets ?: it.cardAssets,
                // 服务端节点已经带上这条玩家输入：回显交给正式消息渲染，避免重复。
                pendingEcho = null,
            )
        }

        loadCardAssets(charId, base, seeded = cached != null)
    }

    /**
     * 拉取（或命中缓存）当前卡的资源表与头像。
     *
     * 只在卡变化时执行一次：会话每次重载都重建资源表既浪费请求，
     * 也会让正文图片在「空表 → 有表」之间反复抖动。
     */
    private fun loadCardAssets(cardId: String, base: String, seeded: Boolean = false) {
        if (cardId.isBlank()) return
        val key = base + '\u0000' + cardId
        if (key == assetsKey) return
        assetsKey = key
        if (seeded) return
        // 换卡时先清掉上一张卡的头像，避免短暂张冠李戴；资源表由下面的请求补齐。
        _state.update { it.copy(characterAvatarUrl = null) }
        viewModelScope.launch {
            val summary = runCatchingCancellable { cardsRepository.assets(cardId) }.getOrNull()
            if (summary == null) {
                assetsKey = "" // 允许下次刷新时重试
                return@launch
            }
            // baseUrl 可能还没从 ServerStore 就绪：这时资源表按服务端实际地址解析，避免拼出空主机。
            val effectiveBase = base.ifBlank { runCatchingCancellable { repository.baseUrl() }.getOrDefault("") }
            val index = dev.tavernagent.mobile.domain.CardAssetResolver.index(summary.assets)
            _state.update {
                it.copy(
                    baseUrl = it.baseUrl.ifBlank { effectiveBase },
                    characterName = summary.name.ifBlank { it.characterName },
                    characterAvatarUrl = resolveAvatar(summary.avatar, index, effectiveBase) ?: it.characterAvatarUrl,
                    cardAssets = summary.assets,
                )
            }
        }
    }

    /**
     * 头像字段可能是 data URI / 绝对 URL / 资源引用 / 纯 ref / 空。
     * 逐级回退到常见资源名；全都找不到时交给首字母徽标
     * （不再猜 `/assets/avatar`，那是必然 404 的请求）。
     */
    private fun resolveAvatar(
        raw: String?,
        index: dev.tavernagent.mobile.domain.CardAssetResolver.AssetIndex,
        base: String,
    ): String? {
        val value = raw?.trim().orEmpty()
        if (value.isNotEmpty()) {
            if (value.startsWith("http://", ignoreCase = true) ||
                value.startsWith("https://", ignoreCase = true) ||
                value.startsWith("data:", ignoreCase = true)
            ) {
                return value
            }
            dev.tavernagent.mobile.domain.CardAssetResolver.resolveIndexed(index, value, base)?.let { return it }
            val ref = value.substringAfterLast('/')
            if (ref.isNotBlank()) {
                dev.tavernagent.mobile.domain.CardAssetResolver.resolveIndexed(index, ref, base)?.let { return it }
            }
        }
        for (candidate in listOf("iconx", "icon", "avatar", "main")) {
            dev.tavernagent.mobile.domain.CardAssetResolver.resolveIndexed(index, candidate, base)?.let { return it }
        }
        return null
    }

    // ---- 发送与推进 ----

    fun onInputChange(value: String) = _state.update { it.copy(inputText = value) }
    fun onNoteChange(value: String) = _state.update { it.copy(inputNote = value) }
    fun toggleNote() = _state.update { it.copy(noteVisible = !it.noteVisible) }
    fun setOptionsMode(mode: String) = _state.update { it.copy(optionsMode = mode) }

    fun send() {
        val snapshot = _state.value
        if (snapshot.viewingHistory) {
            toast("正在浏览历史记录，请先回到最新")
            return
        }
        if (snapshot.busy) return
        val text = snapshot.inputText.trim()
        val note = snapshot.inputNote.trim()
        if (text.isEmpty()) return
        submit(kind = "text", text = text, note = note.takeIf { it.isNotBlank() })
    }

    fun chooseOption(option: OptionDto) {
        val snapshot = _state.value
        if (snapshot.busy || snapshot.viewingHistory) return
        submit(
            kind = "option",
            text = option.text,
            note = null,
            optionRef = dev.tavernagent.mobile.data.remote.dto.OptionRefDto(
                nodeId = snapshot.lastTurnNodeId,
                optionId = option.optionId,
            ),
        )
    }

    private fun submit(
        kind: String,
        text: String,
        note: String?,
        optionRef: dev.tavernagent.mobile.data.remote.dto.OptionRefDto? = null,
    ) {
        val snapshot = _state.value
        val key = pendingKey ?: Keys.new("turn").also {
            pendingKey = it
            pendingHeadId = snapshot.headNodeId
            pendingBranchVersion = snapshot.branchVersion
        }
        val input = TurnInputDto(
            kind = kind,
            text = text,
            note = note,
            options = snapshot.optionsMode.takeIf { it != "auto" },
            optionRef = optionRef,
        )
        pendingInput = input
        viewModelScope.launch {
            _state.update { it.copy(busyAction = "send", error = null) }
            runCatchingCancellable {
                repository.acceptTurn(
                    sessionId = snapshot.sessionId,
                    branchId = snapshot.branchId,
                    idempotencyKey = key,
                    expectedHeadId = snapshot.headNodeId,
                    expectedVersion = snapshot.branchVersion,
                    characterId = snapshot.characterId,
                    input = input,
                )
            }.onSuccess { response ->
                clearPendingKey()
                lastInput = input
                pendingInput = null
                _state.update {
                    it.copy(
                        busyAction = null,
                        inputText = "",
                        inputNote = "",
                        noteVisible = false,
                        draftBlocks = emptyList(),
                        thinking = "",
                        thinkingSeconds = null,
                        // 发送已被受理：立刻把玩家消息挂到消息流，不等整轮生成。
                        pendingEcho = PendingEcho(kind = kind, text = text, note = note),
                    )
                }
                startStream(response.turnId)
            }.onFailure { error ->
                // 4xx（含 429 QUEUE_FULL）表示服务端已明确拒绝：该幂等键不再复用。
                val rejected = error is ApiException && error.status in 400..499
                if (rejected) {
                    clearPendingKey()
                }
                _state.update {
                    it.copy(
                        busyAction = null,
                        phase = TurnPhase.FAILED,
                        error = error.message ?: "发送失败",
                        errorRetryable = error !is ApiException || error.retryable || error.status == 429,
                    )
                }
            }
        }
    }

    /** 失败重试：有未决提交时复用同一幂等键；键已被服务端占用则换新键重发。 */
    fun retry() {
        val pending = pendingInput
        if (pending != null) {
            val snapshot = _state.value
            val key = pendingKey ?: Keys.new("turn").also {
                pendingKey = it
                pendingHeadId = snapshot.headNodeId
                pendingBranchVersion = snapshot.branchVersion
            }
            // 复用意图时捕获的并发基线：head/version 变动也不改变同键载荷。
            val headId = pendingHeadId ?: snapshot.headNodeId
            val version = pendingBranchVersion ?: snapshot.branchVersion
            viewModelScope.launch {
                _state.update { it.copy(busyAction = "retry", error = null) }
                runCatchingCancellable {
                    repository.acceptTurn(
                        sessionId = snapshot.sessionId,
                        branchId = snapshot.branchId,
                        idempotencyKey = key,
                        expectedHeadId = headId,
                        expectedVersion = version,
                        characterId = snapshot.characterId,
                        input = pending,
                    )
                }.onSuccess { response ->
                    clearPendingKey()
                    lastInput = pending
                    pendingInput = null
                    _state.update {
                        it.copy(
                            busyAction = null,
                            inputText = "",
                            inputNote = "",
                            pendingEcho = PendingEcho(kind = pending.kind, text = pending.text, note = pending.note),
                        )
                    }
                    startStream(response.turnId)
                }.onFailure { error ->
                    if (error is ApiException && error.status in 400..499) clearPendingKey()
                    _state.update {
                        it.copy(busyAction = null, phase = TurnPhase.FAILED, error = error.message ?: "重试失败")
                    }
                }
            }
            return
        }
        val snapshot = _state.value
        // 终态失败：provider 已失败、回合不可续，重放同一回合只会立刻再次失败。
        // 用新幂等键重发一个新回合（保留原输入），让"重试"真的能重试。
        val last = lastInput
        if (snapshot.phase == TurnPhase.FAILED && last != null) {
            clearPendingKey()
            submit(kind = last.kind, text = last.text, note = last.note, optionRef = last.optionRef)
            return
        }
        val turnId = snapshot.currentTurnId
        if (turnId != null) {
            viewModelScope.launch {
                runCatchingCancellable { repository.cancelTurn(turnId, _state.value.characterId) }
                startStream(turnId)
            }
        } else {
            toast("没有可重试的内容")
        }
    }

    fun cancel() {
        val turnId = _state.value.currentTurnId ?: return
        viewModelScope.launch {
            runCatchingCancellable { repository.cancelTurn(turnId, _state.value.characterId) }
                .onSuccess {
                    _state.update {
                        val echo = it.pendingEcho
                        it.copy(
                            phase = TurnPhase.IDLE,
                            draftBlocks = emptyList(),
                            thinking = "",
                            // 取消后把玩家消息放回输入框，避免这条内容凭空消失。
                            inputText = it.inputText.ifBlank { echo?.text.orEmpty() },
                            inputNote = it.inputNote.ifBlank { echo?.note.orEmpty() },
                            pendingEcho = null,
                        )
                    }
                }
                .onFailure { error -> _state.update { it.copy(error = error.message ?: "取消失败") } }
        }
    }

    fun continueTurn() {
        val turnId = _state.value.currentTurnId ?: return
        viewModelScope.launch {
            _state.update { it.copy(busyAction = "continue") }
            runCatchingCancellable { repository.continueTurn(turnId, _state.value.characterId) }
                .onSuccess { startStream(turnId) }
                .onFailure { error -> _state.update { it.copy(busyAction = null, error = error.message ?: "续写失败") } }
        }
    }

    // ---- 分支 ----

    fun regenerate(nodeId: String, recheck: Boolean, note: String? = null) {
        val snapshot = _state.value
        viewModelScope.launch {
            _state.update { it.copy(busyAction = "regenerate", error = null) }
            runCatchingCancellable {
                repository.regenerate(snapshot.sessionId, snapshot.branchId, nodeId, snapshot.characterId, recheck, note)
            }.onSuccess { derived ->
                _state.update { it.copy(busyAction = null, branchId = derived.branchId, viewingNodeId = null, draftBlocks = emptyList(), thinking = "") }
                reload(full = true, branchHint = derived.branchId)
                startStream(derived.turnId)
            }.onFailure { error ->
                _state.update { it.copy(busyAction = null, error = error.message ?: "重生成失败") }
            }
        }
    }

    fun fork(nodeId: String) {
        val snapshot = _state.value
        viewModelScope.launch {
            _state.update { it.copy(busyAction = "fork", error = null) }
            runCatchingCancellable {
                repository.fork(snapshot.sessionId, nodeId, snapshot.characterId, name = "平行分支_${System.currentTimeMillis() % 10000}")
            }.onSuccess { branch ->
                _state.update { it.copy(busyAction = null, branchId = branch.branchId, viewingNodeId = null, toast = "已开辟新分支") }
                reload(full = true, branchHint = branch.branchId)
            }.onFailure { error ->
                _state.update { it.copy(busyAction = null, error = error.message ?: "开辟分支失败") }
            }
        }
    }

    fun switchBranch(branchId: String) {
        val snapshot = _state.value
        viewModelScope.launch {
            streamJob?.cancel()
            _state.update {
                it.copy(branchId = branchId, viewingNodeId = null, draftBlocks = emptyList(), thinking = "", phase = TurnPhase.IDLE, currentTurnId = null)
            }
            val view = runCatchingCancellable { repository.view(snapshot.sessionId, branchId, null, 100) }.getOrNull()
            if (view != null) {
                applyView(view)
                repository.rememberLocation(view.sessionId, view.branch.branchId, view.branch.headNodeId, view.title)
                view.branch.activeTurnId?.takeIf { it.isNotBlank() }?.let { startStream(it) }
            }
        }
    }

    fun viewAt(nodeId: String) {
        _state.update { it.copy(viewingNodeId = nodeId) }
        reload(full = true)
    }

    fun backToHead() {
        _state.update { it.copy(viewingNodeId = null) }
        reload(full = true)
    }

    fun loadMoreHistory() {
        val oldest = _state.value.view?.oldestTurnId ?: return
        if (oldest.isBlank() || _state.value.historyLoading) return
        reload(full = false, before = oldest, branchHint = _state.value.branchId)
    }

    fun selectBranchFromView(branchId: String) = switchBranch(branchId)

    // ---- 流式订阅 ----

    private fun startStream(turnId: String) {
        val snapshot = _state.value
        repository.ensureStreaming(
            sessionId = snapshot.sessionId,
            branchId = snapshot.branchId,
            turnId = turnId,
            characterId = snapshot.characterId,
            title = snapshot.title,
        )
        _state.update { it.copy(currentTurnId = turnId, phase = TurnPhase.GENERATING, error = null) }
        streamJob?.cancel()
        streamJob = viewModelScope.launch {
            repository.streamState(turnId).collect { stream ->
                val previousPhase = _state.value.phase
                _state.update {
                    it.copy(
                        draftBlocks = stream.blocks,
                        thinking = stream.thinking,
                        thinkingSeconds = stream.thinkingSeconds,
                        phase = stream.phase,
                        error = stream.error ?: it.error,
                        errorRetryable = true,
                    )
                }
                if (stream.phase == TurnPhase.COMMITTED && previousPhase != TurnPhase.COMMITTED) {
                    if (stream.thinking.isNotBlank()) thinkingByTurn[turnId] = stream.thinking
                    stream.thinkingSeconds?.let { thinkingSecondsByTurn[turnId] = it }
                    reloadAfterCommit(turnId)
                }
            }
        }
    }

    private fun reloadAfterCommit(turnId: String) {
        viewModelScope.launch {
            val view = runCatchingCancellable {
                repository.view(_state.value.sessionId, _state.value.branchId, null, 100)
            }.getOrNull()
            // 网络往返期间用户可能已取消并发出新回合：绝不能用旧回合的视图/终态覆盖新回合。
            if (_state.value.currentTurnId != turnId) return@launch
            if (view != null) {
                // 思考内容从 turnId 迁移到新节点，历史回看时仍可见。
                val head = view.branch.headNodeId
                thinkingByTurn[turnId]?.let { thinkingByTurn[head] = it }
                thinkingSecondsByTurn[turnId]?.let { thinkingSecondsByTurn[head] = it }
                applyView(view)
            }
            repository.forgetStream(turnId)
            _state.update {
                it.copy(
                    phase = TurnPhase.IDLE,
                    currentTurnId = null,
                    draftBlocks = emptyList(),
                    thinking = "",
                    thinkingSeconds = null,
                    error = null,
                )
            }
        }
    }

    fun loadModels() {
        viewModelScope.launch {
            runCatchingCancellable { modelsRepository.catalog() }
                .onSuccess { catalog ->
                    _state.update { it.copy(models = catalog.models, slots = catalog.slots) }
                }
        }
    }

    fun switchPrimaryModel(modelId: String) {
        viewModelScope.launch {
            runCatchingCancellable {
                modelsRepository.saveSlot("primary", enabled = true, modelId = modelId)
            }.onSuccess { updatedSlots ->
                val picked = _state.value.models.firstOrNull { it.id == modelId }
                val label = picked?.name?.ifBlank { picked.model.orEmpty() } ?: modelId
                _state.update {
                    it.copy(
                        slots = updatedSlots,
                        toast = "主线模型已切换为「$label」",
                    )
                }
            }.onFailure { error ->
                _state.update { it.copy(toast = "切换模型失败：${error.message}") }
            }
        }
    }

    fun setReasoningEffort(effort: String) {
        val current = _state.value.activePrimaryModel
        if (current == null) {
            _state.update { it.copy(toast = "请先在「模型设置」中配置连接并指派为对话模型") }
            return
        }
        if (current.reasoningEffort.orEmpty() == effort) return
        viewModelScope.launch {
            runCatchingCancellable {
                modelsRepository.save(current.copy(reasoningEffort = effort, apiKey = ""))
            }.onSuccess { saved ->
                _state.update { state ->
                    val updatedModels = state.models.map { if (it.id == saved.id) saved else it }
                    state.copy(
                        models = updatedModels,
                        toast = "思考强度已设为「${effortLabel(effort)}」",
                    )
                }
            }.onFailure { error ->
                _state.update { it.copy(toast = "设置思考强度失败：${error.message}") }
            }
        }
    }

    fun toast(message: String) = _state.update { it.copy(toast = message) }

    fun clearToast() = _state.update { it.copy(toast = null) }

    fun clearError() = _state.update { it.copy(error = null, phase = if (it.phase == TurnPhase.FAILED) TurnPhase.IDLE else it.phase) }

    override fun onCleared() {
        streamJob?.cancel()
        super.onCleared()
    }

    companion object {
        fun factory() = viewModelFactory {
            initializer {
                val container = TavernApp.instance.container
                StoryViewModel(
                    repository = container.storyRepository,
                    cardsRepository = container.cardsRepository,
                    modelsRepository = container.modelsRepository,
                )
            }
        }
    }
}
