package dev.tavernagent.mobile.domain

import dev.tavernagent.mobile.data.remote.ApiJson
import dev.tavernagent.mobile.data.remote.SseFrame
import dev.tavernagent.mobile.data.remote.dto.BlockAppendedPayload
import dev.tavernagent.mobile.data.remote.dto.BlockDeltaPayload
import dev.tavernagent.mobile.data.remote.dto.TurnFailedPayload
import dev.tavernagent.mobile.data.remote.dto.TurnStartedPayload
import kotlinx.serialization.json.JsonPrimitive

/** 回合事件流语义事件。 */
sealed interface TurnEvent {
    data class Started(val turnId: String) : TurnEvent
    data class ThinkingDelta(val text: String) : TurnEvent
    data class BlockDelta(val seq: Int, val kind: String, val speakerId: String?, val delta: String) : TurnEvent
    data class BlockAppended(val seq: Int, val kind: String, val speakerId: String?, val text: String) : TurnEvent
    data object Truncated : TurnEvent
    data object AwaitingConfirm : TurnEvent
    data class Committed(val nodeId: String?) : TurnEvent
    data object Cancelled : TurnEvent
    data class Failed(val code: String, val message: String) : TurnEvent
    data object Conflicted : TurnEvent
    data class Snapshot(
        val status: String,
        val inFlight: dev.tavernagent.mobile.data.remote.dto.InFlightDto?,
    ) : TurnEvent
    data class Disconnected(val message: String, val retryable: Boolean) : TurnEvent
}

/** SSE 帧 → 语义事件的纯映射（便于单元测试）。 */
object TurnEventCodec {

    private val terminal = setOf("turn.committed", "turn.cancelled", "turn.failed", "turn.conflicted")

    fun isTerminal(frame: SseFrame): Boolean = frame.event in terminal

    fun parse(frame: SseFrame): TurnEvent? = when (frame.event) {
        "turn.started" -> {
            val payload = ApiJson.decodeOrNull<TurnStartedPayload>(frame.data)
            TurnEvent.Started(payload?.turnId.orEmpty())
        }
        "turn.thinking" -> TurnEvent.ThinkingDelta(ApiJson.decodeOrNull<dev.tavernagent.mobile.data.remote.dto.ThinkingPayload>(frame.data)?.thinking.orEmpty())
        "block.delta" -> {
            val payload = ApiJson.decodeOrNull<BlockDeltaPayload>(frame.data) ?: return null
            TurnEvent.BlockDelta(payload.seq, payload.kind, payload.speakerId, payload.delta)
        }
        "block.appended" -> {
            val payload = ApiJson.decodeOrNull<BlockAppendedPayload>(frame.data) ?: return null
            val frameDto = payload.frame ?: return null
            TurnEvent.BlockAppended(frameDto.seq, frameDto.kind, frameDto.speakerId, frameDto.text)
        }
        "turn.truncated" -> TurnEvent.Truncated
        "turn.committed" -> TurnEvent.Committed(committedNodeId(frame.data))
        "turn.cancelled" -> TurnEvent.Cancelled
        "turn.failed" -> {
            val payload = ApiJson.decodeOrNull<TurnFailedPayload>(frame.data)
            TurnEvent.Failed(payload?.code.orEmpty(), payload?.message?.takeIf { it.isNotBlank() } ?: "生成失败")
        }
        "turn.conflicted" -> TurnEvent.Conflicted
        else -> null
    }

    private fun committedNodeId(data: String): String? {
        val element = runCatching { ApiJson.instance.parseToJsonElement(data) }.getOrNull() ?: return null
        val obj = element as? kotlinx.serialization.json.JsonObject ?: return null
        return (obj["nodeId"] as? JsonPrimitive)?.content?.takeIf { it.isNotBlank() }
    }

    /** 把 `GET /turns/{id}` 的状态映射为事件（轮询对账路径）。 */
    fun fromStatus(
        status: String,
        inFlight: dev.tavernagent.mobile.data.remote.dto.InFlightDto? = null,
        failureCode: String? = null,
        failureMessage: String? = null,
    ): TurnEvent? =
        when (status) {
            "committed" -> TurnEvent.Committed(null)
            "cancelled" -> TurnEvent.Cancelled
            // 轮询兜底也要透传服务端失败码/消息，否则用户只看到笼统的"生成失败"。
            "failed" -> TurnEvent.Failed(failureCode.orEmpty(), failureMessage?.takeIf { it.isNotBlank() } ?: "生成失败")
            "conflicted" -> TurnEvent.Conflicted
            "awaiting_continuation" -> TurnEvent.Truncated
            "awaiting_approval" -> TurnEvent.AwaitingConfirm
            else -> TurnEvent.Snapshot(status, inFlight)
        }
}
