package dev.tavernagent.mobile.ui.screens.cards

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import dev.tavernagent.mobile.TavernApp
import dev.tavernagent.mobile.data.remote.dto.OpeningVariantDto
import dev.tavernagent.mobile.data.repo.CardsRepository
import dev.tavernagent.mobile.data.repo.StoryRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * 角色资料：人设预览 + 与这个角色的对话（继续 / 新开）+ 删除角色。
 *
 * 只做展示与转发，剧情与状态一律以服务端为准；「开始新对话」直接调用
 * 既有的建会话接口（开场白变体由服务端给出，客户端不自行拼装）。
 */
class CharacterDetailViewModel(
    private val cards: CardsRepository,
    private val story: StoryRepository,
    private val cardId: String,
) : ViewModel() {

    data class ChatRef(
        val sessionId: String,
        val title: String,
        val updatedAt: String?,
    )

    data class State(
        val loading: Boolean = true,
        val name: String = "",
        val role: String? = null,
        val tags: List<String> = emptyList(),
        val description: String? = null,
        val personality: String? = null,
        val scenario: String? = null,
        val openings: List<OpeningVariantDto> = emptyList(),
        val warnings: List<String> = emptyList(),
        val format: String? = null,
        val avatarUrl: String? = null,
        val chats: List<ChatRef> = emptyList(),
        val busy: Boolean = false,
        val error: String? = null,
        val toast: String? = null,
        val deleted: Boolean = false,
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _state.update { it.copy(loading = true, error = null) }
            runCatching {
                val preview = cards.preview(cardId)
                val avatar = cards.detailAvatar(cardId)
                val chats = runCatching { story.sessions() }.getOrDefault(emptyList())
                    .filter { it.characterId == cardId }
                    .sortedByDescending { it.updatedAt.orEmpty() }
                    .map { ChatRef(sessionId = it.sessionId, title = it.title, updatedAt = it.updatedAt) }
                Triple(preview, avatar, chats)
            }.onSuccess { (preview, avatar, chats) ->
                _state.update {
                    it.copy(
                        loading = false,
                        name = preview.name.ifBlank { "未命名角色" },
                        role = preview.role,
                        tags = preview.tags,
                        description = preview.description,
                        personality = preview.personality,
                        scenario = preview.scenario,
                        openings = preview.openings,
                        warnings = preview.warnings,
                        format = preview.format,
                        avatarUrl = avatar,
                        chats = chats,
                    )
                }
            }.onFailure { error ->
                _state.update { it.copy(loading = false, error = error.message ?: "读取角色失败") }
            }
        }
    }

    fun startChat(
        title: String,
        playerName: String,
        playerRole: String,
        opening: OpeningVariantDto?,
        onCreated: (sessionId: String, branchId: String) -> Unit,
    ) {
        viewModelScope.launch {
            _state.update { it.copy(busy = true, error = null) }
            runCatching {
                story.createSession(
                    cardId = cardId,
                    characterJson = null,
                    title = title.ifBlank { _state.value.name },
                    playerName = playerName.ifBlank { "旅人" },
                    playerRole = playerRole.takeIf { it.isNotBlank() },
                    openingVariantId = opening?.variantId,
                    openingText = opening?.text,
                )
            }.onSuccess { created ->
                _state.update { it.copy(busy = false) }
                onCreated(created.sessionId, created.branchId)
            }.onFailure { error ->
                _state.update { it.copy(busy = false, error = error.message ?: "创建对话失败") }
            }
        }
    }

    fun deleteCard() {
        viewModelScope.launch {
            _state.update { it.copy(busy = true) }
            runCatching { cards.delete(cardId) }
                .onSuccess { _state.update { it.copy(busy = false, deleted = true) } }
                .onFailure { error -> _state.update { it.copy(busy = false, error = error.message ?: "删除失败") } }
        }
    }

    fun clearToast() = _state.update { it.copy(toast = null) }

    companion object {
        fun factory(cardId: String) = viewModelFactory {
            initializer {
                CharacterDetailViewModel(
                    TavernApp.instance.container.cardsRepository,
                    TavernApp.instance.container.storyRepository,
                    cardId,
                )
            }
        }
    }
}
