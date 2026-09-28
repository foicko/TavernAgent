package dev.tavernagent.mobile.data.repo

import dev.tavernagent.mobile.data.local.ServerCredentials
import dev.tavernagent.mobile.data.local.ServerStore
import dev.tavernagent.mobile.data.remote.ApiClient
import dev.tavernagent.mobile.data.remote.ApiProvider
import dev.tavernagent.mobile.data.remote.HttpEngine
import dev.tavernagent.mobile.data.remote.NetworkFailure
import dev.tavernagent.mobile.data.remote.UrlNormalizer
import dev.tavernagent.mobile.data.remote.api.AuthApi
import dev.tavernagent.mobile.util.runCatchingCancellable
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import javax.net.ssl.SSLException

/** 连接检查的结果，UI 直接按分支提示。 */
sealed interface ConnectionCheck {
    data object Connected : ConnectionCheck

    /** 服务器启用了配对码，需要用户输入。 */
    data class NeedsPairPin(val message: String) : ConnectionCheck

    /** 自签证书需要用户确认指纹。[previousTrusted] 表示之前信任过但证书变了。 */
    data class NeedsTrust(
        val hostPort: String,
        val fingerprint: String,
        val previousTrusted: Boolean,
    ) : ConnectionCheck

    data class Failed(val message: String, val retryable: Boolean) : ConnectionCheck
}

/**
 * 连接与配对（功能 1）。
 *
 * 语义与 Web 端一致：先探测 `/api/v1/auth/status`；仅在服务端声明 required 时
 * 才要求配对码。本机访问、或服务端未启用 PIN 时直接通过。
 */
class ConnectionRepository(
    private val serverStore: ServerStore,
    private val apiProvider: ApiProvider,
) {

    val credentials: Flow<ServerCredentials> = serverStore.credentials

    suspend fun savedCredentials(): ServerCredentials = serverStore.credentials.first()

    /** 应用启动时静默恢复：只在已保存地址时使用。 */
    suspend fun restore(): ConnectionCheck {
        val saved = serverStore.credentials.first()
        if (!saved.isConfigured) return ConnectionCheck.Failed("尚未配置服务地址", false)
        return check(saved.baseUrl, saved.token)
    }

    /**
     * 检查连接。不会保存；成功由调用方决定是否 [save]。
     */
    suspend fun check(rawBaseUrl: String, token: String = ""): ConnectionCheck {
        val baseUrl = UrlNormalizer.normalize(rawBaseUrl)
        if (baseUrl.isBlank()) return ConnectionCheck.Failed("请输入服务地址", false)
        val hostPort = UrlNormalizer.hostPort(baseUrl)
            ?: return ConnectionCheck.Failed("地址格式不正确，例如 192.168.1.5:8890", false)

        val credentials = credentialsWithTrust(baseUrl, token)
        return try {
            val status = AuthApi(apiProvider.ephemeral(credentials)).status()
            when {
                status.required && !status.authenticated -> ConnectionCheck.NeedsPairPin("该服务已开启局域网配对，请输入配对码")
                else -> ConnectionCheck.Connected
            }
        } catch (e: NetworkFailure) {
            if (e.isTlsFailure()) trustCheck(baseUrl, token, e)
            else ConnectionCheck.Failed(e.message ?: "无法连接服务", retryable = true)
        } catch (e: Exception) {
            ConnectionCheck.Failed(e.message ?: "无法连接服务", retryable = true)
        }
    }

    /** 用配对码完成配对；成功后保存连接信息。 */
    suspend fun pairAndSave(rawBaseUrl: String, pin: String): ConnectionCheck {
        val baseUrl = UrlNormalizer.normalize(rawBaseUrl)
        if (baseUrl.isBlank()) return ConnectionCheck.Failed("请输入服务地址", false)
        val credentials = credentialsWithTrust(baseUrl, "")
        return try {
            val result = AuthApi(apiProvider.ephemeral(credentials)).pair(pin.trim())
            if (!result.ok) return ConnectionCheck.Failed("配对失败", false)
            val verified = check(baseUrl, result.token)
            if (verified is ConnectionCheck.Connected) {
                save(baseUrl, result.token)
            }
            verified
        } catch (e: Exception) {
            if (e is NetworkFailure && e.isTlsFailure()) trustCheck(baseUrl, "", e)
            else ConnectionCheck.Failed(e.message ?: "配对失败", retryable = true)
        }
    }

    /** 用户确认证书指纹后保存，并重新探测。 */
    suspend fun trustCertificateAndCheck(rawBaseUrl: String, token: String = ""): ConnectionCheck {
        val baseUrl = UrlNormalizer.normalize(rawBaseUrl)
        val hostPort = UrlNormalizer.hostPort(baseUrl)
            ?: return ConnectionCheck.Failed("地址格式不正确", false)
        val key = "${hostPort.first}:${hostPort.second}"
        return try {
            val certificate = HttpEngine.fetchPeerCertificate(hostPort.first, hostPort.second)
            serverStore.saveTrustedCertificate(key, HttpEngine.toDer(certificate))
            check(baseUrl, token)
        } catch (e: Exception) {
            ConnectionCheck.Failed("读取服务器证书失败：${e.message ?: "未知错误"}", retryable = true)
        }
    }

    suspend fun save(baseUrl: String, token: String) = serverStore.saveConnection(baseUrl, token)

    suspend fun forget() = serverStore.clear()

    /** 读取指纹（仅用于展示）。 */
    suspend fun fingerprintOf(baseUrl: String): String? {
        val hostPort = UrlNormalizer.hostPort(baseUrl) ?: return null
        return runCatchingCancellable {
            val certificate = HttpEngine.fetchPeerCertificate(hostPort.first, hostPort.second)
            HttpEngine.sha256Fingerprint(certificate)
        }.getOrNull()
    }

    private suspend fun trustCheck(baseUrl: String, token: String, cause: NetworkFailure): ConnectionCheck {
        val hostPort = UrlNormalizer.hostPort(baseUrl)
            ?: return ConnectionCheck.Failed(cause.message ?: "无法连接服务", true)
        val key = "${hostPort.first}:${hostPort.second}"
        val previouslyTrusted = serverStore.credentials.first().trustedHostPort == key
        val fingerprint = runCatchingCancellable {
            val certificate = HttpEngine.fetchPeerCertificate(hostPort.first, hostPort.second)
            HttpEngine.sha256Fingerprint(certificate)
        }.getOrNull()
            ?: return ConnectionCheck.Failed("TLS 握手失败：${cause.message ?: "证书不受信任"}", true)
        return ConnectionCheck.NeedsTrust(key, fingerprint, previouslyTrusted)
    }

    private suspend fun credentialsWithTrust(baseUrl: String, token: String): ServerCredentials {
        val stored = serverStore.credentials.first()
        val hostPort = UrlNormalizer.hostPort(baseUrl)
        val key = hostPort?.let { "${it.first}:${it.second}" }.orEmpty()
        val reuse = stored.trustedHostPort.isNotBlank() && stored.trustedHostPort == key
        return ServerCredentials(
            baseUrl = baseUrl,
            token = token,
            trustedHostPort = if (reuse) stored.trustedHostPort else "",
            trustedCertDerBase64 = if (reuse) stored.trustedCertDerBase64 else "",
        )
    }
}

/** 沿异常链判断是否 TLS 层失败。 */
fun NetworkFailure.isTlsFailure(): Boolean =
    generateSequence<Throwable>(this) { it.cause }.any { it is SSLException }
