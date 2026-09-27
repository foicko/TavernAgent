package dev.tavernagent.mobile.data.remote.api

import dev.tavernagent.mobile.data.remote.ApiClient
import dev.tavernagent.mobile.data.remote.ApiJson
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

@Serializable
data class AuthStatus(
    val authenticated: Boolean = false,
    val required: Boolean = false,
)

@Serializable
data class PairResult(
    val ok: Boolean = false,
    val token: String = "",
)

/** 鉴权与配对：GET /api/v1/auth/status、POST /api/v1/auth/pair。 */
class AuthApi(private val client: ApiClient) {

    suspend fun status(): AuthStatus = ApiJson.from(client.get("/api/v1/auth/status"))

    suspend fun pair(pin: String): PairResult {
        val body: JsonObject = buildJsonObject { put("pin", pin) }
        return ApiJson.from(client.post("/api/v1/auth/pair", body))
    }
}
