package dev.tavernagent.mobile.ui.screens.chats

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import dev.tavernagent.mobile.TavernApp
import dev.tavernagent.mobile.data.remote.dto.SessionViewDto
import dev.tavernagent.mobile.data.repo.CardsRepository
import dev.tavernagent.mobile.data.repo.StoryRepository
import dev.tavernagent.mobile.domain.StoryMessages
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.util.concurrent.ConcurrentHashMap

/**
 * 聊天列表：每个会话就是一段「与某个角色的对话」。
 *
 * - 标题取角色名（回退到会话标题），头像取角色的轻量资源；
 * - 副标题是最近一条消息摘要：按会话惰性拉取（`limit=1` 的视图，只取分支头），
 *   并按 `updatedAt` 缓存，帖子没更新就不重放历史；
 * - 排序与时间戳都以服务端的 `updatedAt` 为准，不在本地推断。
 */
class ChatsViewModel(
    private val story: StoryRepository,
    private val cards: CardsRepository,
) : ViewModel() {

    data class ChatItem(
        val sessionId: String,
        val characterId: String,
        val name: String,
        val title: String,
        val avatarUrl: String? = null,
        val preview: String? = null,
        val updatedAt: String? = null,
    )

    data class State(
        val loading: Boolean = true,
        val items: List<ChatItem> = emptyList(),
        val error: String? = null,
        val lastSessionId: String = "",
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    /** sessionId → (updatedAt, 摘要)：内容没变就不重复拉取。 */
    private val previewCache = ConcurrentHashMap<String, Pair<String?, String?>>()

    init {
        load()
        viewModelScope.launch {
            story.lastLocation.collect { prefs ->
                _state.update { it.copy(lastSessionId = prefs.lastSessionId) }
            }
        }
    }

    fun load() {
        viewModelScope.launch {
            _state.update { it.copy(loading = it.items.isEmpty(), error = null) }
            val sessions = runCatching { story.sessions() }
                .getOrElse { error ->
                    _state.update { it.copy(loading = false, error = error.message ?: "加载失败") }
                    return@launch
                }
            val cardsById = runCatching { cards.list() }.getOrDefault(emptyList()).associateBy { it.cardId }
            val items = sessions
                .sortedByDescending { it.updatedAt.orEmpty() }
                .map { session ->
                    val card = session.characterId?.let { cardsById[it] }
                    ChatItem(
                        sessionId = session.sessionId,
                        characterId = session.characterId.orEmpty(),
                        name = card?.name?.takeIf { it.isNotBlank() } ?: session.title.ifBlank { "未命名对话" },
                        title = session.title,
                        updatedAt = session.updatedAt,
                    )
                }
            _state.update { it.copy(loading = false, items = items, error = null) }

            fillAvatars(items.mapNotNull { cardsById[it.characterId] })
            fillPreviews(items)
        }
    }

    fun delete(sessionId: String) {
        viewModelScope.launch {
            runCatching { story.deleteSession(sessionId) }
                .onSuccess {
                    previewCache.remove(sessionId)
                    load()
                }
                .onFailure { error -> _state.update { it.copy(error = error.message ?: "删除失败") } }
        }
    }

    fun clearError() = _state.update { it.copy(error = null) }

    private fun fillAvatars(cardsInUse: List<dev.tavernagent.mobile.data.remote.dto.CharacterCardEntryDto>) {
        viewModelScope.launch {
            val entries = cardsInUse.distinctBy { it.cardId }
            val avatars = entries
                .chunked(FETCH_CONCURRENCY)
                .flatMap { chunk ->
                    chunk.map { card -> async { card.cardId to runCatching { cards.listAvatar(card) }.getOrNull() } }
                        .awaitAll()
                }
                .filter { !it.second.isNullOrBlank() }
                .toMap()
            if (avatars.isEmpty()) return@launch
            _state.update { st ->
                st.copy(items = st.items.map { item -> item.copy(avatarUrl = avatars[item.characterId] ?: item.avatarUrl) })
            }
        }
    }

    private fun fillPreviews(items: List<ChatItem>) {
        viewModelScope.launch {
            items.take(PREVIEW_LIMIT)
                .chunked(FETCH_CONCURRENCY)
                .forEach { chunk ->
                    val results = chunk.map { item -> async { item.sessionId to loadPreview(item) } }.awaitAll()
                    val filled = results.filter { it.second != null }.toMap()
                    if (filled.isEmpty()) return@forEach
                    _state.update { st ->
                        st.copy(items = st.items.map { if (filled.containsKey(it.sessionId)) it.copy(preview = filled[it.sessionId]) else it })
                    }
                }
        }
    }

    private suspend fun loadPreview(item: ChatItem): String? {
        previewCache[item.sessionId]?.let { cached ->
            if (cached.first == item.updatedAt) return cached.second
        }
        val preview = runCatching {
            val view = story.view(item.sessionId, limit = 1)
            lastMessagePreview(view)
        }.getOrNull()
        previewCache[item.sessionId] = item.updatedAt to preview
        return preview
    }

    /** 取最后一条非空文本（优先角色发言，其次玩家输入），剥掉标记后截断。 */
    private fun lastMessagePreview(view: SessionViewDto): String? {
        val messages = StoryMessages.fromView(view)
        for (message in messages.asReversed()) {
            val blockText = message.blocks.asReversed().firstOrNull { it.text.isNotBlank() }?.text
            val text = blockText ?: message.inputText.takeIf { it.isNotBlank() }
            if (!text.isNullOrBlank()) return cleanPreview(text)
        }
        return null
    }

    private fun cleanPreview(raw: String): String {
        var text = raw
        text = text.replace(Regex("(?is)<img[^>]*>"), "[图片]")
        text = text.replace(Regex("(?i)\\[IMG:[^\\]\\r\\n]{0,200}\\]"), "[图片]")
        text = text.replace(Regex("(?is)<thought>.*?</thought>"), "")
        text = text.replace(Regex("\\s+"), " ").trim()
        return text.take(PREVIEW_CHARS)
    }

    companion object {
        private const val PREVIEW_LIMIT = 12
        private const val PREVIEW_CHARS = 72
        private const val FETCH_CONCURRENCY = 3

        val factory = viewModelFactory {
            initializer {
                ChatsViewModel(
                    TavernApp.instance.container.storyRepository,
                    TavernApp.instance.container.cardsRepository,
                )
            }
        }
    }
}
