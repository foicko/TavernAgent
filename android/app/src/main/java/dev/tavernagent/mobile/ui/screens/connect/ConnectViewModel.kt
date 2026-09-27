package dev.tavernagent.mobile.ui.screens.connect

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
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class ConnectUiState(
    val address: String = "",
    val pin: String = "",
    val busy: Boolean = false,
    val pinRequired: Boolean = false,
    val trust: ConnectionCheck.NeedsTrust? = null,
    val error: String? = null,
    val errorRetryable: Boolean = true,
    val saved: Boolean = false,
)

/**
 * 连接与配对（功能 1）。成功与否完全由服务端 `/api/v1/auth/status` 决定，
 * 不在本地推断“本机免检”之类的规则。
 */
class ConnectViewModel(private val repository: ConnectionRepository) : ViewModel() {

    private val _state = MutableStateFlow(ConnectUiState())
    val state: StateFlow<ConnectUiState> = _state.asStateFlow()

    var onConnected: (() -> Unit)? = null

    init {
        viewModelScope.launch {
            val saved = repository.savedCredentials()
            _state.update {
                it.copy(
                    address = it.address.ifBlank { saved.baseUrl },
                    saved = saved.isConfigured,
                )
            }
        }
    }

    fun onAddressChange(value: String) = _state.update { it.copy(address = value, error = null) }

    fun onPinChange(value: String) = _state.update { it.copy(pin = value.filter(Char::isDigit), error = null) }

    fun connect() {
        val address = _state.value.address
        _state.update { it.copy(busy = true, error = null, trust = null) }
        viewModelScope.launch {
            val result = repository.check(address, "")
            if (result is ConnectionCheck.Connected) {
                // 服务端未要求配对（本机、或未启用 PIN）时也要记住地址，否则返回后无法复用凭据。
                repository.save(
                    dev.tavernagent.mobile.data.remote.UrlNormalizer.normalize(address),
                    "",
                )
            }
            applyResult(result)
        }
    }

    fun pair() {
        val snapshot = _state.value
        if (snapshot.pin.isBlank()) {
            _state.update { it.copy(error = "请输入配对码") }
            return
        }
        _state.update { it.copy(busy = true, error = null) }
        viewModelScope.launch {
            applyResult(repository.pairAndSave(snapshot.address, snapshot.pin))
        }
    }

    fun trustCertificate() {
        val snapshot = _state.value
        _state.update { it.copy(busy = true, error = null) }
        viewModelScope.launch {
            val result = repository.trustCertificateAndCheck(snapshot.address, "")
            when (result) {
                is ConnectionCheck.Connected -> {
                    repository.save(
                        dev.tavernagent.mobile.data.remote.UrlNormalizer.normalize(snapshot.address),
                        "",
                    )
                    applyResult(result)
                }
                is ConnectionCheck.NeedsPairPin -> applyResult(result)
                else -> applyResult(result)
            }
        }
    }

    fun dismissTrust() = _state.update { it.copy(trust = null) }

    private fun applyResult(result: ConnectionCheck) {
        when (result) {
            is ConnectionCheck.Connected -> {
                _state.update { it.copy(busy = false, error = null, pinRequired = false, trust = null) }
                onConnected?.invoke()
            }
            is ConnectionCheck.NeedsPairPin -> _state.update {
                it.copy(busy = false, pinRequired = true, error = null, trust = null)
            }
            is ConnectionCheck.NeedsTrust -> _state.update {
                it.copy(busy = false, trust = result, error = null)
            }
            is ConnectionCheck.Failed -> _state.update {
                it.copy(busy = false, error = result.message, errorRetryable = result.retryable)
            }
        }
    }

    companion object {
        val Factory = viewModelFactory {
            initializer { ConnectViewModel(TavernApp.instance.container.connectionRepository) }
        }
    }
}
