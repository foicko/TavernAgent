package dev.tavernagent.mobile.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import dev.tavernagent.mobile.TavernApp
import dev.tavernagent.mobile.data.repo.ConnectionCheck
import dev.tavernagent.mobile.data.repo.ConnectionRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** 应用根状态：启动时静默恢复已保存的连接。 */
class RootViewModel(private val repository: ConnectionRepository) : ViewModel() {

    data class DeepLinkTarget(val sessionId: String, val branchId: String)

    sealed interface State {
        data object Restoring : State
        data object NeedsConnect : State
        data object Ready : State
    }

    private val _state = MutableStateFlow<State>(State.Restoring)
    val state: StateFlow<State> = _state.asStateFlow()

    private val _pendingDeepLink = MutableStateFlow<DeepLinkTarget?>(null)
    val pendingDeepLink: StateFlow<DeepLinkTarget?> = _pendingDeepLink.asStateFlow()

    init {
        restore()
    }

    fun restore() {
        viewModelScope.launch {
            _state.value = State.Restoring
            val result = runCatching { repository.restore() }.getOrNull()
            _state.value = if (result is ConnectionCheck.Connected) State.Ready else State.NeedsConnect
        }
    }

    fun onConnected() {
        _state.value = State.Ready
    }

    fun disconnect() {
        viewModelScope.launch {
            runCatching { repository.forget() }
            _state.value = State.NeedsConnect
        }
    }

    fun handleDeepLink(sessionId: String?, branchId: String?) {
        if (!sessionId.isNullOrBlank()) {
            _pendingDeepLink.value = DeepLinkTarget(
                sessionId = sessionId,
                branchId = branchId?.ifBlank { "_" } ?: "_",
            )
        }
    }

    fun consumeDeepLink() {
        _pendingDeepLink.value = null
    }

    companion object {
        val Factory = viewModelFactory {
            initializer { RootViewModel(TavernApp.instance.container.connectionRepository) }
        }
    }
}
