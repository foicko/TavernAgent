package dev.tavernagent.mobile.ui.screens.cards

import android.content.Context
import android.net.Uri
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import dev.tavernagent.mobile.TavernApp
import dev.tavernagent.mobile.data.local.FilePickers
import dev.tavernagent.mobile.data.remote.dto.CharacterCardEntryDto
import dev.tavernagent.mobile.data.repo.CardsRepository
import dev.tavernagent.mobile.data.repo.StoryRepository
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * 角色库：可对话角色的列表、导入与删除。
 *
 * 每行给出角色头像与「已有几段对话」，点进去是角色资料（继续或新开对话）。
 * 角色数据全部来自服务端卡库，客户端不落盘、不改写。
 */
class CardsViewModel(
    private val cards: CardsRepository,
    private val story: StoryRepository,
) : ViewModel() {

    data class State(
        val loading: Boolean = true,
        val cards: List<CharacterCardEntryDto> = emptyList(),
        val avatars: Map<String, String> = emptyMap(),
        val chatCounts: Map<String, Int> = emptyMap(),
        val error: String? = null,
        val busy: Boolean = false,
        val toast: String? = null,
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _state.update { it.copy(loading = it.cards.isEmpty(), error = null) }
            val list = runCatching { cards.list() }
                .getOrElse { error ->
                    _state.update { it.copy(loading = false, error = error.message ?: "加载失败") }
                    return@launch
                }
            _state.update { it.copy(loading = false, cards = list, error = null) }
            fillAvatars(list)
            fillChatCounts()
        }
    }

    fun import(context: Context, uri: Uri) {
        viewModelScope.launch {
            _state.update { it.copy(busy = true, error = null) }
            runCatching {
                val picked = kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.IO) {
                    FilePickers.read(context, uri)
                }
                cards.import(picked.bytes, picked.mediaType)
            }.onSuccess { preview ->
                _state.update { it.copy(busy = false, toast = "已导入「${preview.name}」") }
                load()
            }.onFailure { error ->
                _state.update { it.copy(busy = false, error = error.message ?: "导入失败") }
            }
        }
    }

    fun delete(cardId: String) {
        viewModelScope.launch {
            runCatching { cards.delete(cardId) }
                .onSuccess {
                    _state.update { it.copy(toast = "已删除角色") }
                    load()
                }
                .onFailure { error -> _state.update { it.copy(error = error.message ?: "删除失败") } }
        }
    }

    fun clearToast() = _state.update { it.copy(toast = null) }

    private fun fillAvatars(list: List<CharacterCardEntryDto>) {
        viewModelScope.launch {
            val avatars = list
                .chunked(3)
                .flatMap { chunk ->
                    chunk.map { card -> async { card.cardId to runCatching { cards.listAvatar(card) }.getOrNull() } }
                        .awaitAll()
                }
                .mapNotNull { (cardId, url) -> url?.takeIf { it.isNotBlank() }?.let { cardId to it } }
                .toMap()
            _state.update { it.copy(avatars = avatars) }
        }
    }

    private fun fillChatCounts() {
        viewModelScope.launch {
            val counts = runCatching { story.sessions() }.getOrDefault(emptyList())
                .mapNotNull { it.characterId?.takeIf { id -> id.isNotBlank() } }
                .groupingBy { it }
                .eachCount()
            _state.update { it.copy(chatCounts = counts) }
        }
    }

    companion object {
        fun factory() = viewModelFactory {
            initializer {
                CardsViewModel(
                    TavernApp.instance.container.cardsRepository,
                    TavernApp.instance.container.storyRepository,
                )
            }
        }
    }
}
