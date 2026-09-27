package dev.tavernagent.mobile.data.repo

import dev.tavernagent.mobile.data.remote.ApiProvider
import dev.tavernagent.mobile.data.remote.dto.CardPreviewDto
import dev.tavernagent.mobile.data.remote.dto.CharacterCardEntryDto
import java.util.concurrent.ConcurrentHashMap

/** 角色卡库（服务端为唯一真相；本地只缓存列表用于展示）。 */
class CardsRepository(private val apiProvider: ApiProvider) {

    /**
     * 卡内资源摘要：会话正文渲染只需要名称映射与头像，
     * 不把整份卡 JSON（含 Base64 头像）常驻内存。
     */
    data class CardAssets(
        val cardId: String,
        val name: String,
        val avatar: String?,
        /** 资源名 → 服务端返回的 URL（可能是相对路径，渲染时补 baseUrl）。 */
        val assets: Map<String, String>,
    )

    private val assetCache = ConcurrentHashMap<String, CardAssets>()

    suspend fun list(): List<CharacterCardEntryDto> = apiProvider.current().cards.list()

    suspend fun preview(cardId: String): CardPreviewDto = apiProvider.current().cards.get(cardId)

    /**
     * 卡内资源摘要（带进程内缓存）。
     *
     * 会话每次重载都要用它解析正文图片；卡资源在导入后不可变，按
     * 「服务器地址 + cardId」缓存即可——重开故事不再重复拉几百条 assets，
     * 资源表也能在同一帧内就位，避免「先按裸键 404、再重解析」的空窗。
     */
    suspend fun assets(cardId: String): CardAssets {
        val client = apiProvider.current()
        val key = client.credentials.baseUrl.trimEnd('/') + '\u0000' + cardId
        assetCache[key]?.let { return it }
        val preview = client.cards.get(cardId)
        val summary = CardAssets(
            cardId = cardId,
            name = preview.name,
            avatar = preview.avatar,
            assets = preview.assets
                .filter { it.name.isNotBlank() && it.url.isNotBlank() }
                .associate { it.name to it.url },
        )
        assetCache[key] = summary
        return summary
    }

    /** 进程内已缓存的资源摘要（非挂起：可在渲染同一帧里同步取用）。 */
    fun cachedAssets(cardId: String): CardAssets? {
        if (cardId.isBlank()) return null
        val suffix = '\u0000' + cardId
        return assetCache.entries.firstOrNull { it.key.endsWith(suffix) }?.value
    }

    /** 卡被重新导入 / 删除后必须失效，否则会继续用旧资源表解析。 */
    fun invalidateAssets(cardId: String? = null) {
        if (cardId == null) {
            assetCache.clear()
        } else {
            assetCache.keys.removeIf { it.endsWith('\u0000' + cardId) }
        }
    }

    suspend fun import(bytes: ByteArray, mediaType: String): CardPreviewDto {
        val preview = apiProvider.current().cards.import(bytes, mediaType)
        invalidateAssets()
        return preview
    }

    suspend fun delete(cardId: String) {
        apiProvider.current().cards.delete(cardId)
        invalidateAssets(cardId)
    }

    suspend fun assetUrl(cardId: String, name: String): String =
        apiProvider.current().cards.assetUrl(cardId, name)

    /**
     * 角色头像（列表 / 资料页统一入口）。
     *
     * 优先用资源表里的轻量图标（`iconx` / `icon` / `avatar` / `main`）——
     * charx 卡的 `avatar` 字段常是 MB 级 Base64，长列表里逐行解码会明显卡顿；
     * 找不到图标资源时才回退到卡内 avatar 字段（含 data URI）或引用名。
     */
    suspend fun displayAvatar(cardId: String, rawAvatar: String?): String? {
        runCatching { assets(cardId) }.getOrNull()?.let { summary ->
            val base = apiProvider.current().credentials.baseUrl
            val index = dev.tavernagent.mobile.domain.CardAssetResolver.index(summary.assets)
            for (key in AVATAR_KEYS) {
                dev.tavernagent.mobile.domain.CardAssetResolver.resolveIndexed(index, key, base)?.let { return it }
            }
        }
        val value = rawAvatar?.trim().orEmpty()
        if (value.isEmpty()) return null
        if (value.startsWith("http://", ignoreCase = true) ||
            value.startsWith("https://", ignoreCase = true) ||
            value.startsWith("data:", ignoreCase = true)
        ) {
            return value
        }
        val ref = value.substringAfterLast('/')
        if (ref.isBlank()) return null
        return assetUrlOrNull(cardId, ref)
    }

    /** 列表用的角色头像（从卡片条目出发）。 */
    suspend fun listAvatar(card: CharacterCardEntryDto): String? = displayAvatar(card.cardId, card.avatar)

    /** 资料页用的角色头像（只拿得到 cardId 时）。 */
    suspend fun detailAvatar(cardId: String): String? {
        val summary = runCatching { assets(cardId) }.getOrNull()
        return displayAvatar(cardId, summary?.avatar)
    }

    /** 当前服务器下卡内资源的绝对 URL（供 Coil 加载）。 */
    suspend fun assetUrlOrNull(cardId: String, name: String): String? =
        runCatching { apiProvider.current().cards.assetUrl(cardId, name) }.getOrNull()

    suspend fun avatarUrl(card: CharacterCardEntryDto): String? {
        val avatar = card.avatar ?: return null
        if (avatar.startsWith("http") || avatar.startsWith("data:")) return avatar
        // 卡库头像可能是 /api/v1/cards/{id}/assets/{ref} 或 embeded:// 引用。
        val ref = avatar.substringAfterLast('/')
        if (ref.isBlank()) return null
        return assetUrlOrNull(card.cardId, ref)
    }
}

/** 头像优先级：图标类资源通常是小图，适合列表逐行加载。 */
private val AVATAR_KEYS = listOf("iconx", "icon", "avatar", "main")
