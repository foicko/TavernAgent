package dev.tavernagent.mobile.data.remote.api

import dev.tavernagent.mobile.data.remote.ApiClient
import dev.tavernagent.mobile.data.remote.ApiJson
import dev.tavernagent.mobile.data.remote.dto.BranchDto
import dev.tavernagent.mobile.data.remote.dto.BranchListDto
import dev.tavernagent.mobile.data.remote.dto.CardListDto
import dev.tavernagent.mobile.data.remote.dto.CardPreviewDto
import dev.tavernagent.mobile.data.remote.dto.CharacterCardEntryDto
import dev.tavernagent.mobile.data.remote.dto.CreateSessionResponseDto
import dev.tavernagent.mobile.data.remote.dto.ForkResponseDto
import dev.tavernagent.mobile.data.remote.dto.GraphViewDto
import dev.tavernagent.mobile.data.remote.dto.NodeViewDto
import dev.tavernagent.mobile.data.remote.dto.SessionDto
import dev.tavernagent.mobile.data.remote.dto.SessionImportResponseDto
import dev.tavernagent.mobile.data.remote.dto.SessionListDto
import dev.tavernagent.mobile.data.remote.dto.SessionViewDto
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/** 角色卡库：导入（PNG/JSON/charx 原始字节）、列表、详情、删除、资源 URL。 */
class CardsApi(private val client: ApiClient) {

    suspend fun list(): List<CharacterCardEntryDto> =
        ApiJson.from<CardListDto>(client.get("/api/v1/cards")).cards

    suspend fun import(bytes: ByteArray, mediaType: String): CardPreviewDto =
        ApiJson.from(client.upload("/api/v1/cards", bytes, mediaType))

    suspend fun get(cardId: String): CardPreviewDto =
        ApiJson.from(client.get("/api/v1/cards/$cardId"))

    suspend fun delete(cardId: String) {
        client.delete("/api/v1/cards/$cardId")
    }

    /** 卡内资源是公开 GET（浏览器 <img> 无法带 Token），直接给出绝对 URL 供图片加载器使用。 */
    fun assetUrl(cardId: String, name: String): String =
        client.url("/api/v1/cards/$cardId/assets/$name").toString()
}

@Serializable
data class CreateSessionBodyDto(
    val idempotencyKey: String,
    val title: String,
    val cardId: String? = null,
    val characterJson: String? = null,
    val playerName: String,
    val playerRole: String? = null,
    val openingVariantId: String? = null,
    val openingText: String? = null,
)

/** 会话、分支、视图、图谱与剧情包。 */
class SessionsApi(private val client: ApiClient) {

    suspend fun list(): List<SessionDto> =
        ApiJson.from<SessionListDto>(client.get("/api/v1/sessions")).sessions

    suspend fun create(body: CreateSessionBodyDto): CreateSessionResponseDto =
        ApiJson.from(client.post("/api/v1/sessions", ApiJson.toJson(body)))

    suspend fun view(
        sessionId: String,
        branchId: String? = null,
        viewNodeId: String? = null,
        limit: Int? = null,
        before: String? = null,
    ): SessionViewDto {
        val query = buildMap {
            branchId?.takeIf { it.isNotBlank() }?.let { put("branchId", it) }
            viewNodeId?.takeIf { it.isNotBlank() }?.let { put("viewNodeId", it) }
            limit?.let { put("limit", it.toString()) }
            before?.takeIf { it.isNotBlank() }?.let { put("before", it) }
        }
        return ApiJson.from(client.get("/api/v1/sessions/$sessionId", query))
    }

    suspend fun delete(sessionId: String) {
        client.delete("/api/v1/sessions/$sessionId")
    }

    suspend fun branches(sessionId: String): List<BranchDto> =
        ApiJson.from<BranchListDto>(client.get("/api/v1/sessions/$sessionId/branches")).branches

    suspend fun fork(
        sessionId: String,
        fromNodeId: String,
        name: String?,
        expectedCharacterId: String,
    ): BranchDto {
        val body: JsonObject = buildJsonObject {
            put("fromNodeId", fromNodeId)
            if (!name.isNullOrBlank()) put("name", name)
            put("expectedCharacterId", expectedCharacterId)
        }
        return ApiJson.from<ForkResponseDto>(client.post("/api/v1/sessions/$sessionId/branches", body)).branch
    }

    /** 导出整段故事（.tavernpack，application/zip 字节）。 */
    suspend fun exportBytes(sessionId: String, branchId: String? = null): ByteArray =
        client.downloadBytes(
            "/api/v1/sessions/$sessionId/export",
            branchId?.takeIf { it.isNotBlank() }?.let { mapOf("branchId" to it) } ?: emptyMap(),
        )

    /** 导入故事包（原始 zip 字节）。 */
    suspend fun import(bytes: ByteArray): SessionImportResponseDto =
        ApiJson.from(client.upload("/api/v1/sessions/import", bytes, "application/zip"))

    suspend fun graph(
        sessionId: String,
        nodeId: String? = null,
        branchId: String? = null,
        up: Int = 6,
        down: Int = 2,
    ): GraphViewDto {
        val query = buildMap {
            nodeId?.takeIf { it.isNotBlank() }?.let { put("nodeId", it) }
            branchId?.takeIf { it.isNotBlank() }?.let { put("branchId", it) }
            put("up", up.toString())
            put("down", down.toString())
        }
        return ApiJson.from(client.get("/api/v1/sessions/$sessionId/graph", query))
    }

    suspend fun node(nodeId: String): NodeViewDto = ApiJson.from(client.get("/api/v1/nodes/$nodeId"))
}
