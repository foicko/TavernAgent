package dev.tavernagent.mobile.ui.screens.models

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import dev.tavernagent.mobile.TavernApp
import dev.tavernagent.mobile.data.remote.dto.ModelInstanceDto
import dev.tavernagent.mobile.data.remote.dto.SlotBindingDto
import dev.tavernagent.mobile.data.repo.ModelsRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** 模型连接与三槽位（primary=对话 / assist=导演 / reflection=记忆）。 */
class ModelsViewModel(private val repository: ModelsRepository) : ViewModel() {

    data class State(
        val loading: Boolean = true,
        val models: List<ModelInstanceDto> = emptyList(),
        val slots: List<SlotBindingDto> = emptyList(),
        val error: String? = null,
        val busy: Boolean = false,
        val message: String? = null,
        val editing: ModelInstanceDto? = null,
        val probeResults: Map<String, String> = emptyMap(),
    )

    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()

    init {
        load()
    }

    fun load() {
        viewModelScope.launch {
            _state.update { it.copy(loading = it.models.isEmpty(), error = null) }
            runCatching { repository.catalog() }
                .onSuccess { catalog ->
                    _state.update { it.copy(loading = false, models = catalog.models, slots = catalog.slots, error = null) }
                }
                .onFailure { error ->
                    _state.update { it.copy(loading = false, error = error.message ?: "加载模型配置失败") }
                }
        }
    }

    fun startCreate() = _state.update {
        it.copy(editing = ModelInstanceDto(name = "", kind = "openai-chat"))
    }

    fun startEdit(model: ModelInstanceDto) = _state.update { it.copy(editing = model) }

    fun dismissEditor() = _state.update { it.copy(editing = null) }

    fun save(instance: ModelInstanceDto) {
        viewModelScope.launch {
            _state.update { it.copy(busy = true, error = null) }
            runCatching { repository.save(instance) }
                .onSuccess {
                    _state.update { current -> current.copy(busy = false, editing = null, message = "已保存模型连接") }
                    load()
                }
                .onFailure { error ->
                    _state.update { it.copy(busy = false, error = error.message ?: "保存失败") }
                }
        }
    }

    fun delete(modelId: String) {
        viewModelScope.launch {
            runCatching { repository.delete(modelId) }
                .onSuccess {
                    _state.update { it.copy(message = "已删除模型连接") }
                    load()
                }
                .onFailure { error -> _state.update { it.copy(error = error.message ?: "删除失败") } }
        }
    }

    fun probeModel(model: ModelInstanceDto) {
        if (model.id.isBlank()) return
        viewModelScope.launch {
            _state.update { it.copy(busy = true) }
            runCatching { repository.probeModel(model.id, format = false) }
                .onSuccess { result ->
                    _state.update {
                        it.copy(
                            busy = false,
                            probeResults = it.probeResults + (model.id to (result.message ?: if (result.ok) "连通正常" else "探测失败")),
                            message = if (result.ok) "「${model.name}」连通正常" else "「${model.name}」探测失败",
                        )
                    }
                }
                .onFailure { error ->
                    _state.update {
                        it.copy(
                            busy = false,
                            probeResults = it.probeResults + (model.id to (error.message ?: "探测失败")),
                            message = "探测失败：${error.message}",
                        )
                    }
                }
        }
    }

    fun setSlot(slot: String, enabled: Boolean, modelId: String) {
        viewModelScope.launch {
            _state.update { it.copy(busy = true) }
            runCatching { repository.saveSlot(slot, enabled, modelId) }
                .onSuccess { slots ->
                    _state.update { it.copy(busy = false, slots = slots, message = "已更新用途指派") }
                }
                .onFailure { error ->
                    _state.update { it.copy(busy = false, error = error.message ?: "指派失败") }
                }
        }
    }

    fun probeSlot(slot: String, label: String) {
        viewModelScope.launch {
            _state.update { it.copy(busy = true) }
            runCatching { repository.probeSlot(slot, format = false) }
                .onSuccess { result ->
                    _state.update {
                        it.copy(busy = false, message = if (result.ok) "$label 连通正常" else "$label 探测失败：${result.message.orEmpty()}")
                    }
                }
                .onFailure { error ->
                    _state.update { it.copy(busy = false, message = "$label 探测失败：${error.message}") }
                }
        }
    }

    fun clearMessage() = _state.update { it.copy(message = null) }

    companion object {
        fun factory() = viewModelFactory {
            initializer { ModelsViewModel(TavernApp.instance.container.modelsRepository) }
        }
    }
}
