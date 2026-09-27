package dev.tavernagent.mobile.data.remote.dto

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement

// ---- 回合 ----

@Serializable
data class AcceptTurnResponseDto(
    val turnId: String = "",
    val status: String = "",
    val statusUrl: String? = null,
    val eventsUrl: String? = null,
)

@Serializable
data class DeriveTurnResponseDto(
    val branch: BranchDto? = null,
    val branchId: String = "",
    val turnId: String = "",
    val status: String = "",
    val statusUrl: String? = null,
    val eventsUrl: String? = null,
    val headNodeId: String? = null,
)

@Serializable
data class AssistantEditResponseDto(
    val branch: BranchDto? = null,
    val branchId: String = "",
    val nodeId: String = "",
    val mode: String? = null,
)

@Serializable
data class TurnCapabilitiesDto(
    val canCancel: Boolean = false,
    val canContinue: Boolean = false,
    val canRetry: Boolean = false,
    val terminal: Boolean = false,
)

@Serializable
data class InFlightDto(
    val seq: Int = 0,
    val kind: String = "narration",
    val speakerId: String? = null,
    val text: String = "",
)

@Serializable
data class TurnDraftDto(
    val attemptId: String = "",
    val inFlight: InFlightDto? = null,
)

@Serializable
data class TurnViewDto(
    val turnId: String = "",
    val sessionId: String = "",
    val branchId: String = "",
    val status: String = "",
    val mode: String? = null,
    val input: JsonElement? = null,
    val expectedHeadId: String? = null,
    val expectedVersion: Long = 0,
    val failureCode: String? = null,
    val failureMessage: String? = null,
    val capabilities: TurnCapabilitiesDto = TurnCapabilitiesDto(),
    val resultNodeId: String? = null,
    val resultNode: PlotNodeDto? = null,
    val draft: TurnDraftDto? = null,
)

@Serializable
data class TurnActionResponseDto(
    val turnId: String = "",
    val status: String = "",
    val eventsUrl: String? = null,
)

// ---- SSE 载荷（事件 data 的 JSON）----

@Serializable
data class TurnStartedPayload(
    val turnId: String = "",
    val branchId: String = "",
)

@Serializable
data class ThinkingPayload(
    val thinking: String = "",
    val attemptId: String? = null,
)

@Serializable
data class BlockDeltaPayload(
    val seq: Int = 0,
    val kind: String = "narration",
    val speakerId: String? = null,
    val delta: String = "",
)

@Serializable
data class BlockFrameDto(
    val seq: Int = 0,
    val kind: String = "narration",
    val speakerId: String? = null,
    val text: String = "",
)

@Serializable
data class BlockAppendedPayload(
    val frameSeq: Int = 0,
    val frame: BlockFrameDto? = null,
)

@Serializable
data class TurnFailedPayload(
    val code: String = "",
    val message: String = "",
)

@Serializable
data class TurnCommittedPayload(
    val nodeId: String = "",
    val turnNumber: Int = 0,
    val branchId: String = "",
)

@Serializable
data class SessionUpdatedPayload(
    val sessionId: String = "",
    val branchId: String = "",
)

// ---- 记忆 ----

@Serializable
data class MemoryEvidenceDto(
    val sourceNodeId: String? = null,
    val sourceQuote: String? = null,
    val reasoning: String? = null,
    val confidence: String = "",
    val autoDowngraded: Boolean = false,
)

@Serializable
data class MemoryViewDto(
    val memoryId: String = "",
    val sourceNodeId: String = "",
    val kind: String = "observed",
    val ownerIds: List<String> = emptyList(),
    val content: String = "",
    val entityIds: List<String> = emptyList(),
    val confidence: Double = 0.0,
    val importance: Int = 0,
    val pinned: Boolean = false,
    val supersedes: String? = null,
    val hidden: Boolean = false,
    val evidence: MemoryEvidenceDto? = null,
    val mergedFrom: List<String> = emptyList(),
    val secretId: String? = null,
    val subjectKey: String? = null,
    val createdTurn: Int = 0,
    val validFromTurn: Int = 0,
    val validUntilTurn: Int = 0,
    val effective: Boolean = true,
    val protected: Boolean = false,
)

@Serializable
data class MemoryUsageDto(
    val limit: Int = 0,
    val used: Int = 0,
    val protected: Int = 0,
    val headroom: Int = 0,
    val tier: String = "",
    val reclaimable: Int = 0,
)

@Serializable
data class MemoryPageDto(
    val memories: List<MemoryViewDto> = emptyList(),
    val usage: MemoryUsageDto = MemoryUsageDto(),
    val nodeId: String = "",
    val nextCursor: String? = null,
    val total: Int = 0,
    val counts: Map<String, Int> = emptyMap(),
)

@Serializable
data class MemoryPatchBody(
    val content: String? = null,
    val pinned: Boolean? = null,
    val hidden: Boolean? = null,
    val subjectKey: String? = null,
    val expectedCharacterId: String? = null,
    val expectedHeadId: String? = null,
    val expectedVersion: Long? = null,
    val idempotencyKey: String? = null,
)

@Serializable
data class OrganizePlanDto(
    val operations: List<JsonElement> = emptyList(),
    val usage: MemoryUsageDto? = null,
    val skipped: List<String> = emptyList(),
)
