package dev.tavernagent.mobile

import android.content.Context
import dev.tavernagent.mobile.data.local.PrefsStore
import dev.tavernagent.mobile.data.local.ServerStore
import dev.tavernagent.mobile.data.remote.ApiProvider
import dev.tavernagent.mobile.data.repo.CardsRepository
import dev.tavernagent.mobile.data.repo.ConnectionRepository
import dev.tavernagent.mobile.data.repo.DirectorRepository
import dev.tavernagent.mobile.data.repo.MediaRepository
import dev.tavernagent.mobile.data.repo.MemoryRepository
import dev.tavernagent.mobile.data.repo.ModelsRepository
import dev.tavernagent.mobile.data.repo.StoryRepository
import dev.tavernagent.mobile.data.repo.TurnStreamRegistry
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob

import dev.tavernagent.mobile.data.repo.TurnNotifier

/**
 * 手写依赖容器，避免引入 DI 框架。生命周期与应用一致；网络层与本地存储都在这里组装。
 */
class AppContainer(private val context: Context) {

    val appContext: Context get() = context.applicationContext

    private val applicationScope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    val serverStore: ServerStore by lazy { ServerStore(context) }
    val prefsStore: PrefsStore by lazy { PrefsStore(context) }
    val apiProvider: ApiProvider by lazy { ApiProvider(serverStore) }
    val connectionRepository: ConnectionRepository by lazy {
        ConnectionRepository(serverStore, apiProvider)
    }

    val turnStreams: TurnStreamRegistry by lazy {
        TurnStreamRegistry(apiProvider, applicationScope).apply {
            notifier = TurnNotifier { state, success ->
                dev.tavernagent.mobile.service.NotificationPresenter(appContext).notifyFinished(state, success)
            }
        }
    }

    val cardsRepository: CardsRepository by lazy { CardsRepository(apiProvider) }
    val storyRepository: StoryRepository by lazy {
        StoryRepository(apiProvider, prefsStore, turnStreams)
    }
    val memoryRepository: MemoryRepository by lazy { MemoryRepository(apiProvider) }
    val directorRepository: DirectorRepository by lazy { DirectorRepository(apiProvider) }
    val modelsRepository: ModelsRepository by lazy { ModelsRepository(apiProvider) }
    val mediaRepository: MediaRepository by lazy { MediaRepository(apiProvider) }
}
