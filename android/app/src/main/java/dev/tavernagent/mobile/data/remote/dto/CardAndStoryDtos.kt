package dev.tavernagent.mobile.data.remote.dto

import dev.tavernagent.mobile.data.remote.ApiJson
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement

// ---- 角色卡 ----

@Serializable
data class CharacterCardEntryDto(
    val cardId: String = "",
    val name: String = "",
    val shortName: String? = null,
    val avatar: String? = null,
    val format: String? = null,
    val role: String? = null,
    val contentHash: String? = null,
    val createdAt: String? = null,
    val updatedAt: String? = null,
    val lastUsedAt: String? = null,
)

@Serializable
data class CardListDto(val cards: List<CharacterCardEntryDto> = emptyList())

@Serializable
data class CardAssetDto(
    val name: String = "",
    val type: String = "",
    val url: String = "",
)

@Serializable
data class OpeningVariantDto(
    val variantId: String = "",
    val title: String = "",
    val text: String = "",
)

@Serializable
data class CardPreviewDto(
    val cardId: String? = null,
    val shortName: String? = null,
    val role: String? = null,
    val format: String? = null,
    val specVersion: String? = null,
    val source: String? = null,
    val name: String = "",
    val avatar: String? = null,
    val description: String? = null,
    val personality: String? = null,
    val scenario: String? = null,
    val firstMes: String? = null,
    val mesExample: String? = null,
    val systemPrompt: String? = null,
    val postHistoryInstructions: String? = null,
    val creatorNotes: String? = null,
    val tags: List<String> = emptyList(),
    val creator: String? = null,
    val characterVersion: String? = null,
    val nickname: String? = null,
    val supported: List<String> = emptyList(),
    val ignored: List<String> = emptyList(),
    val warnings: List<String> = emptyList(),
    val characterJson: String? = null,
    val openings: List<OpeningVariantDto> = emptyList(),
    val assets: List<CardAssetDto> = emptyList(),
    val capabilities: JsonElement? = null,
    val varDefaults: JsonElement? = null,
    val themeTokens: JsonElement? = null,
    val sdData: JsonElement? = null,
)

// ---- 会话与分支 ----

@Serializable
data class SessionDto(
    val sessionId: String = "",
    val rootNodeId: String = "",
    val title: String = "",
    val createdAt: String? = null,
    val updatedAt: String? = null,
    val rulesetVersion: String? = null,
    val characterId: String? = null,
)

@Serializable
data class SessionListDto(val sessions: List<SessionDto> = emptyList())

// BranchDto 由 Go 生成（GeneratedDtos.kt），此处不再手写。

@Serializable
data class BranchListDto(val branches: List<BranchDto> = emptyList())

@Serializable
data class ForkResponseDto(val branch: BranchDto = BranchDto())

@Serializable
data class CreateSessionResponseDto(
    val sessionId: String = "",
    val title: String = "",
    val rootNodeId: String = "",
    val branchId: String = "",
    val branchName: String = "",
    val branchVersion: Long = 0,
    val openingText: String = "",
    val state: JsonElement? = null,
)

@Serializable
data class SessionImportResponseDto(
    val sessionId: String = "",
    val title: String = "",
    val rootNodeId: String = "",
    val scope: String? = null,
    val counts: JsonElement? = null,
    val assets: JsonElement? = null,
)

// ---- 节点与正文 ----

@Serializable
data class InlineSegmentDto(
    val type: String = "text",
    val text: String? = null,
    val assetKey: String? = null,
)

@Serializable
data class TextBlockDto(
    val kind: String = "narration",
    val speakerId: String? = null,
    val text: String = "",
    val segments: List<InlineSegmentDto> = emptyList(),
)

@Serializable
data class OptionDto(
    val optionId: String = "",
    val intent: String = "unknown",
    val text: String = "",
    val actionRef: String? = null,
)

@Serializable
data class OptionRefDto(
    val nodeId: String = "",
    val optionId: String = "",
)

@Serializable
data class MoodDto(
    val characterId: String = "",
    val moodCode: String = "",
    val text: String = "",
)

@Serializable
data class UiHintDto(
    val kind: String = "",
    val values: Map<String, String> = emptyMap(),
    val text: String? = null,
)

@Serializable
data class ProvenanceDto(
    val turnId: String = "",
    val attemptId: String = "",
    val mode: String = "",
    val modelConfig: String? = null,
)

@Serializable
data class TurnContentDto(
    val inputKind: String = "text",
    val inputText: String = "",
    val inputNote: String? = null,
    val optionRef: OptionRefDto? = null,
    val actionRef: String? = null,
    val blocks: List<TextBlockDto> = emptyList(),
    val options: List<OptionDto> = emptyList(),
    val mood: MoodDto? = null,
    val checks: List<JsonElement> = emptyList(),
    val changes: List<JsonElement> = emptyList(),
    val injectedMemories: List<JsonElement> = emptyList(),
    val optionsMode: String? = null,
    val suppressedOptions: Int = 0,
    val provenance: ProvenanceDto? = null,
    val uiHints: List<UiHintDto> = emptyList(),
)

@Serializable
data class PlotNodeDto(
    val nodeId: String = "",
    val sessionId: String = "",
    val parentId: String = "",
    val kind: String = "turn",
    val depth: Int = 0,
    val turnNumber: Int = 0,
    val schemaVersion: Int = 0,
    val createdAt: String? = null,
    val contentJson: String = "",
) {
    /** 回合节点的正文载荷；非回合节点或损坏内容返回 null。 */
    fun turnContent(): TurnContentDto? = ApiJson.decodeOrNull(contentJson)
}

// ---- 状态投影 ----
// RelationValueDto / ItemInstanceDto / PromiseDto / CharacterMoodDto / GoalDto /
// SceneDto / CharacterInfoDto / WorldStateDto 由 Go 生成（GeneratedDtos.kt）。

// ---- 视图 ----

@Serializable
data class CapabilityBlockDto(
    val code: String = "",
    val message: String = "",
)

@Serializable
data class SessionCapabilitiesDto(
    val busy: Boolean = false,
    val activeTurnId: String? = null,
    val canSend: Boolean = true,
    val canFork: Boolean = false,
    val atTurnNode: Boolean = false,
    val viewingHistory: Boolean = false,
    val block: CapabilityBlockDto? = null,
)

@Serializable
data class SecretViewDto(
    val secretId: String = "",
    val title: String? = null,
    val order: Int = 0,
    val content: String? = null,
    val revealed: Boolean = false,
)

@Serializable
data class OutlineEntryDto(
    val milestoneId: String = "",
    val description: String = "",
)

@Serializable
data class GoalEntryDto(
    val goalId: String = "",
    val characterId: String = "",
    val text: String = "",
)

@Serializable
data class OutlineViewDto(
    val milestones: List<OutlineEntryDto> = emptyList(),
    val goals: List<GoalEntryDto> = emptyList(),
    val scene: String? = null,
)

@Serializable
data class SummaryArtifactDto(
    val summaryId: String = "",
    val fromNodeId: String = "",
    val toNodeId: String = "",
    val sourceHash: String? = null,
    val visibilityScope: String? = null,
    val text: String = "",
    val modelConfigVersion: String? = null,
    val summaryVersion: Int = 0,
)

@Serializable
data class ActionViewDto(
    val actionId: String = "",
    val label: String = "",
    val attribute: String = "",
    val dc: Int = 0,
    val itemIds: List<String> = emptyList(),
)

@Serializable
data class SessionViewDto(
    val sessionId: String = "",
    val characterId: String = "",
    val title: String = "",
    val rootNodeId: String = "",
    val branch: BranchDto = BranchDto(),
    val branches: List<BranchDto> = emptyList(),
    val viewNodeId: String = "",
    val viewNode: PlotNodeDto? = null,
    val headNode: PlotNodeDto? = null,
    val nodes: List<PlotNodeDto> = emptyList(),
    val candidateGroups: Map<String, List<PlotNodeDto>> = emptyMap(),
    val state: JsonElement? = null,
    val secrets: List<SecretViewDto> = emptyList(),
    val outline: OutlineViewDto? = null,
    val activeSummary: SummaryArtifactDto? = null,
    val actions: List<ActionViewDto> = emptyList(),
    val hasMore: Boolean = false,
    val oldestTurnId: String? = null,
    val capabilities: SessionCapabilitiesDto = SessionCapabilitiesDto(),
    val director: DirectorSummaryDto? = null,
)

@Serializable
data class NodeViewDto(
    val node: PlotNodeDto? = null,
    val state: JsonElement? = null,
    val candidates: List<PlotNodeDto> = emptyList(),
)

// ---- 分支图谱 ----

@Serializable
data class GraphNodeDto(
    val nodeId: String = "",
    val parentId: String? = null,
    val kind: String = "turn",
    val turnNumber: Int = 0,
    val depth: Int = 0,
    val childCount: Int = 0,
    val isCandidate: Boolean = false,
)

@Serializable
data class GraphViewDto(
    val sessionId: String = "",
    val targetNodeId: String = "",
    val nodes: List<GraphNodeDto> = emptyList(),
    val truncated: Boolean = false,
)
