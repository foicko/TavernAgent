package dev.tavernagent.mobile.data.repo

import dev.tavernagent.mobile.data.local.PrefsStore
import dev.tavernagent.mobile.data.local.UserPrefs
import dev.tavernagent.mobile.data.remote.ApiProvider
import dev.tavernagent.mobile.data.remote.api.AcceptTurnBodyDto
import dev.tavernagent.mobile.data.remote.api.CreateSessionBodyDto
import dev.tavernagent.mobile.data.remote.api.EditTurnBodyDto
import dev.tavernagent.mobile.data.remote.api.RegenerateBodyDto
import dev.tavernagent.mobile.data.remote.api.TurnInputDto
import dev.tavernagent.mobile.data.remote.dto.AcceptTurnResponseDto
import dev.tavernagent.mobile.data.remote.dto.AssistantEditResponseDto
import dev.tavernagent.mobile.data.remote.dto.BranchDto
import dev.tavernagent.mobile.data.remote.dto.CreateSessionResponseDto
import dev.tavernagent.mobile.data.remote.dto.DeriveTurnResponseDto
import dev.tavernagent.mobile.data.remote.dto.GraphViewDto
import dev.tavernagent.mobile.data.remote.dto.NodeViewDto
import dev.tavernagent.mobile.data.remote.dto.SessionDto
import dev.tavernagent.mobile.data.remote.dto.SessionImportResponseDto
import dev.tavernagent.mobile.data.remote.dto.SessionViewDto
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.StateFlow
import java.util.concurrent.atomic.AtomicLong

/** 幂等键生成：同一语义的重试必须复用同一个键（由调用方持有）。 */
object Keys {
    private val counter = AtomicLong(0)
    fun new(prefix: String): String = "${prefix}_${System.currentTimeMillis()}_${counter.incrementAndGet()}"
}

/**
 * 故事读写入口：会话、分支、回合受理与流式订阅。
 * 所有能力判定以服务端 `capabilities` 为准，客户端不自行推断。
 */
class StoryRepository(
    private val apiProvider: ApiProvider,
    private val prefs: PrefsStore,
    private val turns: TurnStreamRegistry,
) {

    val lastLocation: Flow<UserPrefs> get() = prefs.prefs

    suspend fun sessions(): List<SessionDto> = apiProvider.current().sessions.list()

    suspend fun view(
        sessionId: String,
        branchId: String? = null,
        viewNodeId: String? = null,
        limit: Int? = null,
        before: String? = null,
    ): SessionViewDto = apiProvider.current().sessions.view(sessionId, branchId, viewNodeId, limit, before)

    suspend fun branches(sessionId: String): List<BranchDto> = apiProvider.current().sessions.branches(sessionId)

    suspend fun node(nodeId: String): NodeViewDto = apiProvider.current().sessions.node(nodeId)

    suspend fun graph(sessionId: String, nodeId: String?, branchId: String?, up: Int = 6, down: Int = 2): GraphViewDto =
        apiProvider.current().sessions.graph(sessionId, nodeId, branchId, up, down)

    suspend fun createSession(
        cardId: String?,
        characterJson: String?,
        title: String,
        playerName: String,
        playerRole: String?,
        openingVariantId: String?,
        openingText: String?,
    ): CreateSessionResponseDto {
        val body = CreateSessionBodyDto(
            idempotencyKey = Keys.new("cs"),
            title = title,
            cardId = cardId,
            characterJson = characterJson,
            playerName = playerName,
            playerRole = playerRole,
            openingVariantId = openingVariantId,
            openingText = openingText,
        )
        return apiProvider.current().sessions.create(body)
    }

    suspend fun deleteSession(sessionId: String) = apiProvider.current().sessions.delete(sessionId)

    suspend fun acceptTurn(
        sessionId: String,
        branchId: String,
        idempotencyKey: String,
        expectedHeadId: String,
        expectedVersion: Long,
        characterId: String,
        input: TurnInputDto,
        afterTurnId: String? = null,
    ): AcceptTurnResponseDto {
        val body = AcceptTurnBodyDto(
            idempotencyKey = idempotencyKey,
            expectedHeadId = expectedHeadId,
            expectedVersion = expectedVersion,
            afterTurnId = afterTurnId,
            input = input,
            expectedCharacterId = characterId.ifBlank { null },
        )
        return apiProvider.current().turns.accept(sessionId, branchId, body)
    }

    suspend fun regenerate(
        sessionId: String,
        branchId: String,
        nodeId: String,
        characterId: String,
        recheck: Boolean,
        note: String? = null,
        idempotencyKey: String = Keys.new("rg"),
    ): DeriveTurnResponseDto = apiProvider.current().turns.regenerate(
        sessionId,
        branchId,
        RegenerateBodyDto(
            nodeId = nodeId,
            recheck = recheck,
            idempotencyKey = idempotencyKey,
            note = note?.takeIf { it.isNotBlank() },
            expectedCharacterId = characterId.ifBlank { null },
        ),
    )

    suspend fun editPlayerInput(
        sessionId: String,
        branchId: String,
        nodeId: String,
        characterId: String,
        text: String,
    ): DeriveTurnResponseDto = apiProvider.current().turns.editPlayerInput(
        sessionId,
        branchId,
        EditTurnBodyDto(
            nodeId = nodeId,
            input = TurnInputDto(kind = "text", text = text),
            idempotencyKey = Keys.new("ed"),
            expectedCharacterId = characterId.ifBlank { null },
        ),
    )

    suspend fun editAssistantText(
        sessionId: String,
        branchId: String,
        nodeId: String,
        characterId: String,
        blocks: List<dev.tavernagent.mobile.data.remote.dto.TextBlockDto>,
    ): AssistantEditResponseDto = apiProvider.current().turns.editAssistantText(
        sessionId,
        branchId,
        EditTurnBodyDto(
            nodeId = nodeId,
            blocks = blocks,
            idempotencyKey = Keys.new("ea"),
            expectedCharacterId = characterId.ifBlank { null },
        ),
    )

    suspend fun cancelTurn(turnId: String, characterId: String) =
        apiProvider.current().turns.cancel(turnId, characterId)

    suspend fun continueTurn(turnId: String, characterId: String) =
        apiProvider.current().turns.continueTurn(turnId, characterId)

    suspend fun fork(sessionId: String, fromNodeId: String, characterId: String, name: String? = null): BranchDto =
        apiProvider.current().sessions.fork(sessionId, fromNodeId, name, characterId)

    suspend fun exportSession(sessionId: String, branchId: String? = null): ByteArray =
        apiProvider.current().sessions.exportBytes(sessionId, branchId)

    suspend fun importSession(bytes: ByteArray): SessionImportResponseDto =
        apiProvider.current().sessions.import(bytes)

    suspend fun rememberLocation(sessionId: String, branchId: String, nodeId: String, title: String = "") =
        prefs.setLastLocation(sessionId, branchId, nodeId, title)

    fun streamState(turnId: String): StateFlow<TurnStreamState> = turns.state(turnId)

    suspend fun baseUrl(): String = apiProvider.current().credentials.baseUrl

    fun ensureStreaming(sessionId: String, branchId: String, turnId: String, characterId: String, title: String) =
        turns.ensureStreaming(sessionId, branchId, turnId, characterId, title)

    fun forgetStream(turnId: String) = turns.forget(turnId)
}
