package dev.tavernagent.mobile.data.repo

import dev.tavernagent.mobile.data.remote.ApiProvider
import dev.tavernagent.mobile.data.remote.dto.MemoryPageDto
import dev.tavernagent.mobile.data.remote.dto.MemoryPatchBody
import dev.tavernagent.mobile.data.remote.dto.MemoryViewDto
import dev.tavernagent.mobile.data.remote.dto.OrganizePlanDto

/**
 * 记忆库：查询与写操作（修订/置顶/隐藏/整理）。
 * 写操作是分支级 copy-on-write，必须带分支与幂等键。
 */
class MemoryRepository(private val apiProvider: ApiProvider) {

    suspend fun page(
        sessionId: String,
        branchId: String,
        nodeId: String? = null,
        search: String? = null,
        kind: String? = null,
        limit: Int = 40,
        cursor: String? = null,
    ): MemoryPageDto = apiProvider.current().memories.list(sessionId, branchId, nodeId, search, kind, limit, cursor)

    suspend fun patch(
        sessionId: String,
        branchId: String,
        memoryId: String,
        characterId: String,
        expectedHeadId: String,
        expectedVersion: Long,
        content: String? = null,
        pinned: Boolean? = null,
        hidden: Boolean? = null,
        subjectKey: String? = null,
    ): MemoryViewDto {
        val body = MemoryPatchBody(
            content = content,
            pinned = pinned,
            hidden = hidden,
            subjectKey = subjectKey,
            expectedCharacterId = characterId.ifBlank { null },
            expectedHeadId = expectedHeadId.ifBlank { null },
            expectedVersion = expectedVersion,
            idempotencyKey = Keys.new("mp"),
        )
        return apiProvider.current().memories.patch(sessionId, branchId, memoryId, body)
    }

    suspend fun organize(sessionId: String, branchId: String, characterId: String): OrganizePlanDto =
        apiProvider.current().memories.organize(sessionId, branchId, characterId)
}
