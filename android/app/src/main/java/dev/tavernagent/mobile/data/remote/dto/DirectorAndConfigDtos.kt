package dev.tavernagent.mobile.data.remote.dto

import kotlinx.serialization.Serializable

// ---- 导演 ----
// DirectorBeatDto / DirectorPlanDto / DirectorBeatProgressDto / DirectorStateDto /
// DirectorSummaryDto 由 Go 生成（GeneratedDtos.kt），此处不再手写。

@Serializable
data class DirectorDraftDto(
    val sessionId: String = "",
    val branchId: String = "",
    val version: Long = 0,
    val baseRevisionId: String = "",
    val plan: DirectorPlanDto = DirectorPlanDto(),
    val updatedAt: String? = null,
)

@Serializable
data class DirectorRequestDto(
    val requestId: String = "",
    val sessionId: String = "",
    val branchId: String = "",
    val baseNodeId: String = "",
    val draftVersion: Long = 0,
    val text: String = "",
    val status: String = "generating",
    val reply: String? = null,
    val candidate: DirectorPlanDto? = null,
    val draftApplied: Boolean = false,
    val error: String? = null,
    val createdAt: String? = null,
)

@Serializable
data class DirectorViewDto(
    val state: DirectorStateDto? = null,
    val draft: DirectorDraftDto? = null,
    val requests: List<DirectorRequestDto> = emptyList(),
    val readOnly: Boolean = false,
    val viewNodeId: String? = null,
)

@Serializable
data class DirectorCommandResultDto(
    val nodeId: String = "",
    val version: Long = 0,
    val state: DirectorStateDto? = null,
)

@Serializable
data class DirectorMessageResponseDto(
    val requestId: String = "",
    val status: String = "",
    val statusUrl: String? = null,
    val eventsUrl: String? = null,
)

@Serializable
data class DirectorMessageRequestDto(
    val expectedCharacterId: String,
    val expectedHeadId: String,
    val expectedDraftVersion: Long,
    val idempotencyKey: String,
    val text: String,
)

@Serializable
data class SaveDirectorDraftRequestDto(
    val expectedCharacterId: String,
    val expectedDraftVersion: Long,
    val baseRevisionId: String,
    val plan: DirectorPlanDto,
)

@Serializable
data class DirectorCommandRequestDto(
    val expectedCharacterId: String,
    val expectedHeadId: String,
    val expectedVersion: Long,
    val idempotencyKey: String,
    val action: String,
    val beatId: String? = null,
    val draftVersion: Long = 0,
    val replace: Boolean = false,
)

// ---- 模型配置 ----
// ModelInstanceDto 由 Go 生成（GeneratedDtos.kt），此处不再手写。

@Serializable
data class SlotBindingDto(
    val slot: String = "",
    val enabled: Boolean = false,
    val modelId: String? = null,
    val resolvedModelId: String? = null,
    val model: String? = null,
    val kind: String? = null,
    val baseUrl: String? = null,
    val hasApiKey: Boolean = false,
    val maxTokens: Int = 0,
    val contextWindow: Int = 0,
)

@Serializable
data class ConfigCatalogDto(
    val models: List<ModelInstanceDto> = emptyList(),
    val slots: List<SlotBindingDto> = emptyList(),
)

@Serializable
data class ProvidersResponseDto(val providers: List<SlotBindingDto> = emptyList())

@Serializable
data class SaveSlotRequestDto(
    val slot: String,
    val enabled: Boolean,
    val modelId: String = "",
)

@Serializable
data class ProbeRequestDto(
    val slot: String = "",
    val modelId: String? = null,
    val format: Boolean = false,
)

@Serializable
data class ProbeResultDto(
    val ok: Boolean = false,
    val message: String? = null,
    val latencyMs: Long = 0,
    val model: String? = null,
    val format: Boolean = false,
    val detail: String? = null,
)

// ---- 语音与图像 ----

@Serializable
data class VoiceDto(
    val id: String = "",
    val name: String = "",
    val gender: String = "",
    val locale: String = "",
    val description: String = "",
)

@Serializable
data class VoicesResponseDto(val voices: List<VoiceDto> = emptyList())

@Serializable
data class TtsRequestDto(
    val voice: String,
    val text: String,
    val instruction: String? = null,
    val engine: String? = null,
    val baseUrl: String? = null,
    val apiKey: String? = null,
    val model: String? = null,
    val speed: Double? = null,
)

@Serializable
data class ImageGenerateOptionsDto(
    val engine: String? = null,
    val prompt: String,
    val negativePrompt: String? = null,
    val size: String? = null,
    val baseUrl: String? = null,
    val apiKey: String? = null,
    val model: String? = null,
    val quality: String? = null,
    val style: String? = null,
)

@Serializable
data class GeneratedImageDto(
    val id: String = "",
    val url: String = "",
    val prompt: String = "",
    val mimeType: String = "",
    val createdAt: String = "",
)
