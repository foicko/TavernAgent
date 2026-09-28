package dev.tavernagent.mobile

import android.app.Activity
import android.app.Application
import android.os.Bundle
import coil.ImageLoader
import coil.ImageLoaderFactory
import coil.disk.DiskCache
import coil.memory.MemoryCache
import dev.tavernagent.mobile.data.repo.TurnNotifier
import dev.tavernagent.mobile.domain.TurnPhase
import dev.tavernagent.mobile.media.DataUriFetcher
import dev.tavernagent.mobile.service.GenerationNotifications
import dev.tavernagent.mobile.service.NotificationPresenter

/**
 * 应用入口。持有依赖容器；所有网络状态和凭据都保存在 ServerStore 中。
 */
class TavernApp : Application(), ImageLoaderFactory {

    val container: AppContainer by lazy { AppContainer(this) }

    override fun onCreate() {
        super.onCreate()
        instance = this
        // 渠道只在这里建一次（前台服务与完成通知共用，避免 importance 冲突）。
        GenerationNotifications.ensureChannel(this)
        // 完成通知的唯一 owner：应用在前台时抑制（界面上已经看得到结果）。
        val presenter = NotificationPresenter(this)
        container.turnStreams.notifier = TurnNotifier { state, success ->
            if (success || state.phase == TurnPhase.FAILED) {
                presenter.notifyFinished(state, success)
            }
        }
        registerActivityLifecycleCallbacks(foregroundTracker)
    }

    private val foregroundTracker = object : ActivityLifecycleCallbacks {
        override fun onActivityStarted(activity: Activity) {
            foregroundActivities++
        }

        override fun onActivityStopped(activity: Activity) {
            foregroundActivities = (foregroundActivities - 1).coerceAtLeast(0)
        }

        override fun onActivityCreated(activity: Activity, savedInstanceState: Bundle?) = Unit
        override fun onActivityResumed(activity: Activity) = Unit
        override fun onActivityPaused(activity: Activity) = Unit
        override fun onActivitySaveInstanceState(activity: Activity, outState: Bundle) = Unit
        override fun onActivityDestroyed(activity: Activity) = Unit
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

        /** 可见 Activity 计数（onActivityStarted/Stopped 维护）；用于抑制前台时的完成通知。 */
        @Volatile
        private var foregroundActivities = 0

        val isForeground: Boolean get() = foregroundActivities > 0
    }
}
