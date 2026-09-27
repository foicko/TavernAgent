package dev.tavernagent.mobile.data.remote

import dev.tavernagent.mobile.data.local.ServerCredentials
import dev.tavernagent.mobile.data.local.ServerStore
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * 按当前凭据缓存 [ApiClient]。凭据变化（重新配对、信任新证书）时自动重建。
 */
class ApiProvider(private val serverStore: ServerStore) : ApiClientSource {

    private val mutex = Mutex()
    private var cached: Pair<String, ApiClient>? = null

    override suspend fun current(): ApiClient {
        val credentials = serverStore.credentials.first()
        val key = listOf(
            credentials.baseUrl,
            credentials.token,
            credentials.trustedHostPort,
            credentials.trustedCertDerBase64,
        ).joinToString("|")
        mutex.withLock {
            cached?.takeIf { it.first == key }?.let { return it.second }
            val created = ApiClient(credentials)
            cached = key to created
            return created
        }
    }

    /** 探测/配对等临时连接：不入缓存，避免覆盖已保存的凭据。 */
    fun ephemeral(credentials: ServerCredentials): ApiClient = ApiClient(credentials)
}
