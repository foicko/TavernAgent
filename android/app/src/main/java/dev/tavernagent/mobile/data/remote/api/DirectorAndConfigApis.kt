package dev.tavernagent.mobile.data.remote.api

import dev.tavernagent.mobile.data.remote.ApiClient
import dev.tavernagent.mobile.data.remote.ApiJson
import dev.tavernagent.mobile.data.remote.SseFrame
import dev.tavernagent.mobile.data.remote.dto.ConfigCatalogDto
import dev.tavernagent.mobile.data.remote.dto.DirectorCommandRequestDto
import dev.tavernagent.mobile.data.remote.dto.DirectorCommandResultDto
import dev.tavernagent.mobile.data.remote.dto.DirectorDraftDto
import dev.tavernagent.mobile.data.remote.dto.DirectorMessageRequestDto
import dev.tavernagent.mobile.data.remote.dto.DirectorMessageResponseDto
import dev.tavernagent.mobile.data.remote.dto.DirectorRequestDto
import dev.tavernagent.mobile.data.remote.dto.DirectorViewDto
import dev.tavernagent.mobile.data.remote.dto.GeneratedImageDto
import dev.tavernagent.mobile.data.remote.dto.ImageGenerateOptionsDto
import dev.tavernagent.mobile.data.remote.dto.ModelInstanceDto
import dev.tavernagent.mobile.data.remote.dto.ProbeRequestDto
import dev.tavernagent.mobile.data.remote.dto.ProbeResultDto
import dev.tavernagent.mobile.data.remote.dto.ProvidersResponseDto
import dev.tavernagent.mobile.data.remote.dto.SaveDirectorDraftRequestDto
import dev.tavernagent.mobile.data.remote.dto.SaveSlotRequestDto
import dev.tavernagent.mobile.data.remote.dto.SlotBindingDto
import dev.tavernagent.mobile.data.remote.dto.TtsRequestDto
import dev.tavernagent.mobile.data.remote.dto.VoiceDto
import dev.tavernagent.mobile.data.remote.dto.VoicesResponseDto
import kotlinx.coroutines.flow.Flow
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

@Serializable
private data class ExpectedCharacterBody(val expectedCharacterId: String)

/** 导演模式：查看、草稿、讨论（SSE 终态）、命令。 */
class DirectorApi(private val client: ApiClient) {

    suspend fun view(sessionId: String, branchId: String, viewNodeId: String? = null): DirectorViewDto {
        val query = viewNodeId?.takeIf { it.isNotBlank() }?.let { mapOf("viewNodeId" to it) } ?: emptyMap()
        return ApiJson.from(client.get("/api/v1/sessions/$sessionId/branches/$branchId/director", query))
    }

    suspend fun saveDraft(
        sessionId: String,
        branchId: String,
        body: SaveDirectorDraftRequestDto,
    ): DirectorDraftDto =
        ApiJson.from(
            client.put(
                "/api/v1/sessions/$sessionId/branches/$branchId/director/draft",
                ApiJson.toJson(body),
            ),
        )

    suspend fun command(
        sessionId: String,
        branchId: String,
        body: DirectorCommandRequestDto,
    ): DirectorCommandResultDto =
        ApiJson.from(
            client.post(
                "/api/v1/sessions/$sessionId/branches/$branchId/director/commands",
                ApiJson.toJson(body),
            ),
        )

    suspend fun discuss(
        sessionId: String,
        branchId: String,
        body: DirectorMessageRequestDto,
    ): DirectorMessageResponseDto =
        ApiJson.from(
            client.post(
                "/api/v1/sessions/$sessionId/branches/$branchId/director/messages",
                ApiJson.toJson(body),
            ),
        )

    suspend fun request(requestId: String): DirectorRequestDto =
        ApiJson.from(client.get("/api/v1/director-requests/$requestId"))

    suspend fun cancelRequest(requestId: String, expectedCharacterId: String): DirectorRequestDto {
        val body = buildJsonObject { put("expectedCharacterId", expectedCharacterId) }
        return ApiJson.from(client.post("/api/v1/director-requests/$requestId/cancel", body))
    }

    fun events(requestId: String, lastEventId: String? = null): Flow<SseFrame> =
        client.stream("/api/v1/director-requests/$requestId/events", lastEventId = lastEventId)
}

/** 模型实例与三槽位指派。 */
class ModelsApi(private val client: ApiClient) {

    suspend fun catalog(): ConfigCatalogDto = ApiJson.from(client.get("/api/v1/config/models"))

    suspend fun saveModel(instance: ModelInstanceDto): ModelInstanceDto =
        if (instance.id.isBlank()) {
            ApiJson.from(client.post("/api/v1/config/models", ApiJson.toJson(instance)))
        } else {
            ApiJson.from(client.put("/api/v1/config/models/${instance.id}", ApiJson.toJson(instance)))
        }

    suspend fun deleteModel(modelId: String) {
        client.delete("/api/v1/config/models/$modelId")
    }

    suspend fun probeModel(modelId: String, format: Boolean = false): ProbeResultDto =
        ApiJson.from(
            client.post(
                "/api/v1/config/models/$modelId/probe",
                ApiJson.toJson(ProbeRequestDto(format = format)),
            ),
        )

    suspend fun providers(): List<SlotBindingDto> =
        ApiJson.from<ProvidersResponseDto>(client.get("/api/v1/config/provider")).providers

    suspend fun saveSlot(slot: String, enabled: Boolean, modelId: String): List<SlotBindingDto> =
        ApiJson.from<ProvidersResponseDto>(
            client.put("/api/v1/config/provider", ApiJson.toJson(SaveSlotRequestDto(slot, enabled, modelId))),
        ).providers

    suspend fun probeProvider(slot: String, format: Boolean = false): ProbeResultDto =
        ApiJson.from(
            client.post(
                "/api/v1/config/provider/probe",
                ApiJson.toJson(ProbeRequestDto(slot = slot, format = format)),
            ),
        )
}

/** 语音合成与插画生成（凭据由客户端直传后端，不落服务端存储）。 */
class MediaApi(private val client: ApiClient) {

    suspend fun voices(): List<VoiceDto> =
        ApiJson.from<VoicesResponseDto>(client.get("/api/v1/tts/voices")).voices

    suspend fun synthesize(request: TtsRequestDto): ByteArray =
        client.postForBytes("/api/v1/tts/synthesize", ApiJson.toJson(request).toString())

    suspend fun generateImage(options: ImageGenerateOptionsDto): GeneratedImageDto =
        ApiJson.from(client.post("/api/v1/images/generate", ApiJson.toJson(options)))

    fun absoluteUrl(path: String): String =
        if (path.startsWith("http")) path else client.url(path).toString()
}
