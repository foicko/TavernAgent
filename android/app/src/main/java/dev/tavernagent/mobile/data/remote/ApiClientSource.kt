package dev.tavernagent.mobile.data.remote

/** 提供"当前生效的客户端"的抽象：生产实现按已保存凭据缓存，测试可注入固定客户端。 */
fun interface ApiClientSource {
    suspend fun current(): ApiClient
}
