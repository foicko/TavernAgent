package dev.tavernagent.mobile.data.remote.api

import dev.tavernagent.mobile.data.remote.ApiClient
import dev.tavernagent.mobile.data.remote.ApiJson
import dev.tavernagent.mobile.data.remote.SseFrame
import dev.tavernagent.mobile.data.remote.dto.AcceptTurnResponseDto
import dev.tavernagent.mobile.data.remote.dto.AssistantEditResponseDto
import dev.tavernagent.mobile.data.remote.dto.DeriveTurnResponseDto
import dev.tavernagent.mobile.data.remote.dto.MemoryPageDto
import dev.tavernagent.mobile.data.remote.dto.MemoryPatchBody
import dev.tavernagent.mobile.data.remote.dto.MemoryViewDto
import dev.tavernagent.mobile.data.remote.dto.OptionRefDto
import dev.tavernagent.mobile.data.remote.dto.OrganizePlanDto
import dev.tavernagent.mobile.data.remote.dto.TextBlockDto
import dev.tavernagent.mobile.data.remote.dto.TurnActionResponseDto
import dev.tavernagent.mobile.data.remote.dto.TurnViewDto
import kotlinx.coroutines.flow.Flow
import kotlinx.serialization.Serializable

@Serializable
data class TurnInputDto(
    val kind: String = "text",
    val text: String = "",
    val note: String? = null,
    val options: String? = null,
    val optionRef: OptionRefDto? = null,
    val actionRef: String? = null,
)

@Serializable
data class AcceptTurnBodyDto(
    val idempotencyKey: String,
    val expectedHeadId: String,
    val expectedVersion: Long,
    val afterTurnId: String? = null,
    val mode: String? = null,
    val input: TurnInputDto,
    val expectedCharacterId: String? = null,
)

@Serializable
data class RegenerateBodyDto(
    val nodeId: String,
    val recheck: Boolean = false,
    val idempotencyKey: String,
    val name: String? = null,
    val note: String? = null,
    val options: String? = null,
    val expectedCharacterId: String? = null,
)

@Serializable
data class EditTurnBodyDto(
    val nodeId: String,
    val input: TurnInputDto? = null,
    val blocks: List<TextBlockDto>? = null,
    val idempotencyKey: String,
    val name: String? = null,
    val expectedCharacterId: String? = null,
)

@Serializable
private data class CancelBodyDto(val expectedCharacterId: String)

/** 回合：受理、状态、流式事件、取消/续写、重生成与编辑。 */
class TurnsApi(private val client: ApiClient) {

    suspend fun accept(sessionId: String, branchId: String, body: AcceptTurnBodyDto): AcceptTurnResponseDto =
        ApiJson.from(
            client.post(
                "/api/v1/sessions/$sessionId/branches/$branchId/turns",
                ApiJson.toJson(body),
            ),
        )

    suspend fun get(turnId: String): TurnViewDto = ApiJson.from(client.get("/api/v1/turns/$turnId"))

    suspend fun cancel(turnId: String, expectedCharacterId: String): TurnActionResponseDto =
        ApiJson.from(client.post("/api/v1/turns/$turnId/cancel", ApiJson.toJson(CancelBodyDto(expectedCharacterId))))

    suspend fun continueTurn(turnId: String, expectedCharacterId: String): TurnActionResponseDto =
        ApiJson.from(client.post("/api/v1/turns/$turnId/continue", ApiJson.toJson(CancelBodyDto(expectedCharacterId))))

    suspend fun regenerate(sessionId: String, branchId: String, body: RegenerateBodyDto): DeriveTurnResponseDto =
        ApiJson.from(
            client.post(
                "/api/v1/sessions/$sessionId/branches/$branchId/regenerations",
                ApiJson.toJson(body),
            ),
        )

    suspend fun editPlayerInput(sessionId: String, branchId: String, body: EditTurnBodyDto): DeriveTurnResponseDto =
        ApiJson.from(
            client.post("/api/v1/sessions/$sessionId/branches/$branchId/edits", ApiJson.toJson(body)),
        )

    suspend fun editAssistantText(sessionId: String, branchId: String, body: EditTurnBodyDto): AssistantEditResponseDto =
        ApiJson.from(
            client.post("/api/v1/sessions/$sessionId/branches/$branchId/edits", ApiJson.toJson(body)),
        )

    /** 回合事件流；[lastEventId] 形如 `turnId:seq`，用于断线续传。 */
    fun events(turnId: String, lastEventId: String? = null): Flow<SseFrame> =
        client.stream("/api/v1/turns/$turnId/events", lastEventId = lastEventId)

    /** 会话事件流：只关心 session.updated。 */
    fun sessionEvents(sessionId: String, lastEventId: String? = null): Flow<SseFrame> =
        client.stream("/api/v1/sessions/$sessionId/events", lastEventId = lastEventId)
}

/** 记忆库：分页查询、修订（CoW）、整理。 */
class MemoriesApi(private val client: ApiClient) {

    suspend fun list(
        sessionId: String,
        branchId: String? = null,
        nodeId: String? = null,
        search: String? = null,
        kind: String? = null,
        limit: Int = 40,
        cursor: String? = null,
    ): MemoryPageDto {
        val query = buildMap {
            branchId?.takeIf { it.isNotBlank() }?.let { put("branchId", it) }
            nodeId?.takeIf { it.isNotBlank() }?.let { put("nodeId", it) }
            search?.takeIf { it.isNotBlank() }?.let { put("search", it) }
            kind?.takeIf { it.isNotBlank() }?.let { put("kind", it) }
            put("limit", limit.toString())
            cursor?.takeIf { it.isNotBlank() }?.let { put("cursor", it) }
        }
        return ApiJson.from(client.get("/api/v1/sessions/$sessionId/memories", query))
    }

    suspend fun patch(
        sessionId: String,
        branchId: String,
        memoryId: String,
        body: MemoryPatchBody,
    ): MemoryViewDto =
        ApiJson.from(
            client.patch("/api/v1/sessions/$sessionId/branches/$branchId/memories/$memoryId", ApiJson.toJson(body)),
        )

    suspend fun organize(sessionId: String, branchId: String, expectedCharacterId: String): OrganizePlanDto =
        ApiJson.from(
            client.post(
                "/api/v1/sessions/$sessionId/branches/$branchId/memories/organize",
                ApiJson.toJson(CancelBodyDto(expectedCharacterId)),
            ),
        )
}
