package dev.tavernagent.mobile

import android.app.Application
import coil.ImageLoader
import coil.ImageLoaderFactory
import coil.disk.DiskCache
import coil.memory.MemoryCache
import dev.tavernagent.mobile.data.repo.TurnNotifier
import dev.tavernagent.mobile.media.DataUriFetcher
import dev.tavernagent.mobile.service.NotificationPresenter

/**
 * 应用入口。持有依赖容器；所有网络状态和凭据都保存在 ServerStore 中。
 */
class TavernApp : Application(), ImageLoaderFactory {

    val container: AppContainer by lazy { AppContainer(this) }

    override fun onCreate() {
        super.onCreate()
        instance = this
        val presenter = NotificationPresenter(this)
        container.turnStreams.notifier = TurnNotifier { state, success ->
            if (success || state.phase == dev.tavernagent.mobile.domain.TurnPhase.FAILED) {
                presenter.notifyFinished(state, success)
            }
        }
    }

    /**
     * 卡内立绘会按会话长期复用，必须落盘缓存：
     * - 服务端资源带 `Cache-Control: immutable`，`respectCacheHeaders(false)` 让缓存策略完全由客户端决定；
     * - 显式给出内存 / 磁盘配额（一张立绘 100~600KB），重开故事与翻看历史应零网络请求。
     */
    override fun newImageLoader(): ImageLoader {
        return ImageLoader.Builder(this)
            .components {
                add(DataUriFetcher.StringFactory())
                add(DataUriFetcher.UriFactory())
            }
            .memoryCache {
                MemoryCache.Builder(this)
                    .maxSizePercent(0.25)
                    .build()
            }
            .diskCache {
                DiskCache.Builder()
                    .directory(cacheDir.resolve("image_cache"))
                    .maxSizeBytes(256L * 1024 * 1024)
                    .build()
            }
            .respectCacheHeaders(false)
            .crossfade(true)
            .build()
    }

    companion object {
        lateinit var instance: TavernApp
            private set
    }
}
