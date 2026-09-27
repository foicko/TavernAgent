package dev.tavernagent.mobile.data.repo

import dev.tavernagent.mobile.data.remote.ApiProvider
import dev.tavernagent.mobile.data.remote.SseFrame
import dev.tavernagent.mobile.data.remote.dto.DirectorCommandRequestDto
import dev.tavernagent.mobile.data.remote.dto.DirectorCommandResultDto
import dev.tavernagent.mobile.data.remote.dto.DirectorMessageRequestDto
import dev.tavernagent.mobile.data.remote.dto.DirectorMessageResponseDto
import dev.tavernagent.mobile.data.remote.dto.DirectorPlanDto
import dev.tavernagent.mobile.data.remote.dto.DirectorRequestDto
import dev.tavernagent.mobile.data.remote.dto.DirectorViewDto
import dev.tavernagent.mobile.data.remote.dto.SaveDirectorDraftRequestDto
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.emitAll

/** 导演模式：草稿、讨论（异步终态）与命令。 */
class DirectorRepository(private val apiProvider: ApiProvider) {

    suspend fun view(sessionId: String, branchId: String, viewNodeId: String? = null): DirectorViewDto =
        apiProvider.current().director.view(sessionId, branchId, viewNodeId)

    suspend fun saveDraft(
        sessionId: String,
        branchId: String,
        characterId: String,
        expectedDraftVersion: Long,
        baseRevisionId: String,
        plan: DirectorPlanDto,
    ) = apiProvider.current().director.saveDraft(
        sessionId,
        branchId,
        SaveDirectorDraftRequestDto(
            expectedCharacterId = characterId,
            expectedDraftVersion = expectedDraftVersion,
            baseRevisionId = baseRevisionId,
            plan = plan,
        ),
    )

    /** 导演命令；action ∈ activate | pause | resume | complete | skip | rewind。 */
    suspend fun command(
        sessionId: String,
        branchId: String,
        characterId: String,
        expectedHeadId: String,
        expectedVersion: Long,
        action: String,
        beatId: String? = null,
        draftVersion: Long = 0,
        replace: Boolean = false,
    ): DirectorCommandResultDto = apiProvider.current().director.command(
        sessionId,
        branchId,
        DirectorCommandRequestDto(
            expectedCharacterId = characterId,
            expectedHeadId = expectedHeadId,
            expectedVersion = expectedVersion,
            idempotencyKey = Keys.new("dc"),
            action = action,
            beatId = beatId,
            draftVersion = draftVersion,
            replace = replace,
        ),
    )

    suspend fun discuss(
        sessionId: String,
        branchId: String,
        characterId: String,
        expectedHeadId: String,
        expectedDraftVersion: Long,
        text: String,
    ): DirectorMessageResponseDto = apiProvider.current().director.discuss(
        sessionId,
        branchId,
        DirectorMessageRequestDto(
            expectedCharacterId = characterId,
            expectedHeadId = expectedHeadId,
            expectedDraftVersion = expectedDraftVersion,
            idempotencyKey = Keys.new("dm"),
            text = text,
        ),
    )

    suspend fun request(requestId: String): DirectorRequestDto = apiProvider.current().director.request(requestId)

    suspend fun cancelRequest(requestId: String, characterId: String): DirectorRequestDto =
        apiProvider.current().director.cancelRequest(requestId, characterId)

    fun events(requestId: String): Flow<SseFrame> = kotlinx.coroutines.flow.flow {
        emitAll(apiProvider.current().director.events(requestId))
    }
}
