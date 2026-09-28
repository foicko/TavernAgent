package dev.tavernagent.mobile.data.local

import android.content.Context
import android.util.Base64
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

private val Context.serverDataStore: DataStore<Preferences> by preferencesDataStore(name = "tavern_server")

/** 已保存的连接信息：地址、配对 Token、以及用户确认过的服务器证书（TOFU）。 */
data class ServerCredentials(
    val baseUrl: String = "",
    val token: String = "",
    val trustedHostPort: String = "",
    val trustedCertDerBase64: String = "",
) {
    val isConfigured: Boolean get() = baseUrl.isNotBlank()
    val hasTrustedCert: Boolean get() = trustedCertDerBase64.isNotBlank()

    fun trustedCertDer(): ByteArray? =
        if (trustedCertDerBase64.isBlank()) null
        else runCatching { Base64.decode(trustedCertDerBase64, Base64.NO_WRAP) }.getOrNull()
}

class ServerStore(private val context: Context) {

    val credentials: Flow<ServerCredentials> = context.serverDataStore.data.map { prefs ->
        ServerCredentials(
            baseUrl = prefs[KEY_BASE_URL].orEmpty(),
            // Token 与 TOFU 证书落盘前经 Keystore 加密；读取时解密（历史明文值原样返回）。
            token = SecretCipher.decrypt(prefs[KEY_TOKEN].orEmpty()),
            trustedHostPort = prefs[KEY_TRUSTED_HOST].orEmpty(),
            trustedCertDerBase64 = SecretCipher.decrypt(prefs[KEY_TRUSTED_CERT].orEmpty()),
        )
    }

    suspend fun saveConnection(baseUrl: String, token: String) {
        context.serverDataStore.edit { prefs ->
            prefs[KEY_BASE_URL] = baseUrl
            prefs[KEY_TOKEN] = SecretCipher.encrypt(token)
        }
    }

    suspend fun saveTrustedCertificate(hostPort: String, der: ByteArray) {
        context.serverDataStore.edit { prefs ->
            prefs[KEY_TRUSTED_HOST] = hostPort
            prefs[KEY_TRUSTED_CERT] = SecretCipher.encrypt(Base64.encodeToString(der, Base64.NO_WRAP))
        }
    }

    suspend fun clear() {
        context.serverDataStore.edit { prefs ->
            prefs.remove(KEY_BASE_URL)
            prefs.remove(KEY_TOKEN)
            prefs.remove(KEY_TRUSTED_HOST)
            prefs.remove(KEY_TRUSTED_CERT)
        }
    }

    private companion object {
        val KEY_BASE_URL = stringPreferencesKey("base_url")
        val KEY_TOKEN = stringPreferencesKey("token")
        val KEY_TRUSTED_HOST = stringPreferencesKey("trusted_host_port")
        val KEY_TRUSTED_CERT = stringPreferencesKey("trusted_cert_der_base64")
    }
}
