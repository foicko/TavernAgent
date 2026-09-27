package dev.tavernagent.mobile.data.repo

import dev.tavernagent.mobile.data.remote.ApiClientSource
import dev.tavernagent.mobile.data.remote.ApiException
import dev.tavernagent.mobile.domain.DraftBlock
import dev.tavernagent.mobile.domain.DraftReducer
import dev.tavernagent.mobile.domain.TurnEvent
import dev.tavernagent.mobile.domain.TurnEventCodec
import dev.tavernagent.mobile.domain.TurnPhase
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.channelFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.util.concurrent.ConcurrentHashMap

/** 一次在途回合的流式状态（进程内共享；杀进程后由服务端 activeTurnId 恢复）。 */
data class TurnStreamState(
    val turnId: String = "",
    val sessionId: String = "",
    val branchId: String = "",
    val characterId: String = "",
    val sessionTitle: String = "",
    val phase: TurnPhase = TurnPhase.IDLE,
    val thinking: String = "",
    val thinkingStartedAt: Long = 0L,
    val thinkingSeconds: Int? = null,
    val blocks: List<DraftBlock> = emptyList(),
    val error: String? = null,
) {
    val busy: Boolean get() = phase.isBusy
    val text: String get() = blocks.joinToString("\n\n") { it.text }.trim()
}

/** 回合完成的本地通知出口（由前台服务实现，避免仓库依赖 Android 框架）。 */
fun interface TurnNotifier {
    fun onTurnFinished(state: TurnStreamState, success: Boolean)
}

/**
 * 回合流的唯一持有者：连接、重连、轮询对账与状态归并。
 *
 * 与 Web 端语义一致：
 * - SSE 增量（`block.delta`）即时合并；
 * - 每 3 秒用 `GET /turns/{id}` 对账（断线期间的行内增量只能靠快照补齐，且只允许变长）；
 * - **断线不等于失败**：网络异常 / 5xx / 429 / 服务端提前关流都按 1/2/4/8/16 秒退避后
 *   无限重连，直到拿到终态事件；只有明确的服务端 4xx（429 除外）才终止并报错。
 *   首字/整轮延迟可达数十秒，弱网下一次抖动不该让用户在界面上看到假失败。
 * - 终态事件终止订阅。
 */
class TurnStreamRegistry(
    private val apiSource: ApiClientSource,
    private val scope: CoroutineScope,
    private val reconcileIntervalMs: Long = 3_000,
    private val backoffMs: LongArray = longArrayOf(1_000, 2_000, 4_000, 8_000, 16_000),
) {

    private val states = ConcurrentHashMap<String, MutableStateFlow<TurnStreamState>>()
    private val jobs = ConcurrentHashMap<String, Job>()
    private val mutex = Mutex()

    var notifier: TurnNotifier? = null

    fun state(turnId: String): StateFlow<TurnStreamState> =
        states.getOrPut(turnId) { MutableStateFlow(TurnStreamState(turnId = turnId)) }.asStateFlow()

    fun hasActiveStream(turnId: String): Boolean = jobs[turnId]?.isActive == true

    /** 幂等：同一回合已有活动流时不重复连接。 */
    fun ensureStreaming(
        sessionId: String,
        branchId: String,
        turnId: String,
        characterId: String,
        sessionTitle: String,
    ) {
        if (jobs[turnId]?.isActive == true) return
        val state = states.getOrPut(turnId) { MutableStateFlow(TurnStreamState(turnId = turnId)) }
        state.update {
            it.copy(
                turnId = turnId,
                sessionId = sessionId,
                branchId = branchId,
                characterId = characterId,
                sessionTitle = sessionTitle.ifBlank { it.sessionTitle },
            )
        }
        jobs[turnId] = scope.launch {
            events(turnId, snapshotFirst = state.value.blocks.isEmpty() && state.value.thinking.isEmpty())
                .collect { event -> reduce(state, event) }
        }
    }

    /** 停止本地订阅（不取消服务端回合）。 */
    suspend fun detach(turnId: String) {
        mutex.withLock {
            jobs.remove(turnId)?.cancel()
        }
    }

    fun forget(turnId: String) {
        jobs.remove(turnId)?.cancel()
        states.remove(turnId)
    }

    fun activeTurnIds(): List<String> = jobs.filterValues { it.isActive }.keys.toList()

    /** 合并事件到状态；终态触发通知。 */
    private suspend fun reduce(state: MutableStateFlow<TurnStreamState>, event: TurnEvent) {
        val current = state.value
        val next = when (event) {
            is TurnEvent.Started -> current.copy(phase = TurnPhase.GENERATING)
            is TurnEvent.ThinkingDelta -> current.copy(
                phase = TurnPhase.GENERATING,
                thinking = current.thinking + event.text,
                thinkingStartedAt = current.thinkingStartedAt.takeIf { it > 0 } ?: System.currentTimeMillis(),
            )
            is TurnEvent.BlockDelta -> current.copy(
                phase = TurnPhase.GENERATING,
                blocks = DraftReducer.applyDelta(current.blocks, event.seq, event.kind, event.speakerId, event.delta),
            )
            is TurnEvent.BlockAppended -> current.copy(
                phase = TurnPhase.GENERATING,
                blocks = DraftReducer.applyBlockAppended(current.blocks, event.seq, event.kind, event.speakerId, event.text),
            )
            is TurnEvent.Snapshot -> current.copy(
                phase = when (event.status) {
                    "generating", "preparing", "queued", "accepted", "validating" -> TurnPhase.GENERATING
                    else -> current.phase
                },
                blocks = DraftReducer.applyInFlight(current.blocks, event.inFlight),
                thinkingStartedAt = current.thinkingStartedAt.takeIf { it > 0 } ?: System.currentTimeMillis(),
            )
            TurnEvent.Truncated -> current.copy(phase = TurnPhase.TRUNCATED)
            TurnEvent.AwaitingConfirm -> current.copy(phase = TurnPhase.AWAITING_CONFIRM)
            is TurnEvent.Committed -> current.copy(
                phase = TurnPhase.COMMITTED,
                thinkingSeconds = thinkingSecondsOf(current),
            )
            TurnEvent.Cancelled -> current.copy(phase = TurnPhase.IDLE, blocks = emptyList(), thinking = "", error = null)
            is TurnEvent.Failed -> current.copy(
                phase = TurnPhase.FAILED,
                error = event.message.ifBlank { "生成失败" },
            )
            TurnEvent.Conflicted -> current.copy(phase = TurnPhase.FAILED, error = "分支已变化，请刷新后再试")
            is TurnEvent.Disconnected -> current.copy(
                phase = if (current.phase == TurnPhase.COMMITTED) current.phase else TurnPhase.FAILED,
                error = event.message,
            )
        }
        state.value = next
        when (event) {
            is TurnEvent.Committed -> {
                notifier?.onTurnFinished(next, true)
            }
            is TurnEvent.Failed, TurnEvent.Conflicted -> notifier?.onTurnFinished(next, false)
            is TurnEvent.Disconnected -> if (next.phase == TurnPhase.FAILED) notifier?.onTurnFinished(next, false)
            TurnEvent.Cancelled -> notifier?.onTurnFinished(next, false)
            else -> Unit
        }
    }

    private fun thinkingSecondsOf(state: TurnStreamState): Int? {
        val started = state.thinkingStartedAt
        if (started <= 0L || state.thinking.isBlank()) return null
        return ((System.currentTimeMillis() - started) / 1000).toInt().coerceAtLeast(0)
    }

    /**
     * 事件流：SSE 实时增量 + 定时对账；终态或退避耗尽后结束。
     */
    fun events(turnId: String, snapshotFirst: Boolean): Flow<TurnEvent> = channelFlow {
        val done = CompletableDeferred<Unit>()
        // 仅当 SSE 当前没有在投递增量时才用轮询快照补齐：避免"快照已含后文 + 增量再次追加"的重复。
        var sseActive = false

        launch {
            var wait = if (snapshotFirst) 0L else reconcileIntervalMs
            while (isActive && !done.isCompleted) {
                if (wait > 0) delay(wait)
                wait = reconcileIntervalMs
                val turn = runCatching { apiSource.current().turns.get(turnId) }.getOrNull() ?: continue
                val inFlight = if (sseActive) null else turn.draft?.inFlight
                val event = TurnEventCodec.fromStatus(turn.status, inFlight)
                if (event != null) send(event)
                if (turn.capabilities.terminal) {
                    done.complete(Unit)
                    return@launch
                }
            }
        }

        launch {
            var attempts = 0
            var lastEventId: String? = null
            while (isActive && !done.isCompleted) {
                try {
                    apiSource.current().turns.events(turnId, lastEventId).collect { frame ->
                        sseActive = true
                        frame.id?.let { lastEventId = it }
                        val event = TurnEventCodec.parse(frame) ?: return@collect
                        // 只有「有实质进展」的事件才清零退避：turn.started 常被反复重放，
                        // 拿它当健康信号会让"连上即断"退化成重连风暴。纯心跳式空转逐步退避到上限。
                        if (event !is TurnEvent.Started) attempts = 0
                        send(event)
                        if (TurnEventCodec.isTerminal(frame)) {
                            done.complete(Unit)
                            return@collect
                        }
                    }
                    // 服务端主动关流且未给终态（或连接被中间设备切断）：不是失败，退避后重连。
                    sseActive = false
                    if (isActive && !done.isCompleted) {
                        attempts++
                        delay(backoff(attempts))
                    }
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    sseActive = false
                    val apiError = e as? ApiException
                    // 4xx（429 除外，如 401/404）是明确的服务端拒绝，不是断线：终止并报错。
                    // 其余（网络异常 / 5xx / 429）一律视为可恢复的断线，无限重连而不判失败。
                    val fatal = apiError != null && apiError.status in 400..499 && apiError.status != 429
                    if (fatal) {
                        send(TurnEvent.Disconnected(apiError.message, retryable = apiError.retryable))
                        done.complete(Unit)
                        return@launch
                    }
                    attempts++
                    delay(backoff(attempts))
                }
            }
        }

        try {
            done.await()
        } finally {
            // channelFlow 结束时自动取消子协程
        }
    }

    private fun backoff(retry: Int): Long = backoffMs[(retry - 1).coerceIn(0, backoffMs.lastIndex)]
}
