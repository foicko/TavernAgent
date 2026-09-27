package dev.tavernagent.mobile.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import dev.tavernagent.mobile.MainActivity
import dev.tavernagent.mobile.R
import dev.tavernagent.mobile.TavernApp
import dev.tavernagent.mobile.domain.TurnPhase
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.launch
import dev.tavernagent.mobile.data.repo.TurnStreamState

/**
 * 前台生成服务：应用切后台/锁屏后保持进程与 SSE 连接，完成后发系统通知。
 *
 * 真正的流由进程内的 [dev.tavernagent.mobile.data.repo.TurnStreamRegistry] 持有；
 * 这里只负责前台保活、通知进度与终态提示。进程被杀后，重进应用通过
 * `branch.activeTurnId` + `GET /turns/{id}` 恢复，与 Web 端一致。
 */
class GenerationService : Service() {

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private var progressJob: Job? = null
    private var turnId: String = ""

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        createChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val action = intent?.action
        if (action == ACTION_STOP) {
            stopSelf()
            return START_NOT_STICKY
        }
        val sessionId = intent?.getStringExtra(EXTRA_SESSION).orEmpty()
        val branchId = intent?.getStringExtra(EXTRA_BRANCH).orEmpty()
        val title = intent?.getStringExtra(EXTRA_TITLE).orEmpty()
        val newTurnId = intent?.getStringExtra(EXTRA_TURN).orEmpty()
        if (newTurnId.isBlank()) {
            stopSelf()
            return START_NOT_STICKY
        }
        this.sessionId = sessionId
        this.branchId = branchId
        startForeground(NOTIFICATION_ID, buildNotification(title, "正在演绎…", ongoing = true))
        if (newTurnId != turnId) {
            turnId = newTurnId
            TavernApp.instance.container.turnStreams.ensureStreaming(
                sessionId = sessionId,
                branchId = branchId,
                turnId = newTurnId,
                characterId = "",
                sessionTitle = title,
            )
            observeTurn(newTurnId, title)
        }
        return START_STICKY
    }

    private fun observeTurn(turnId: String, title: String) {
        progressJob?.cancel()
        progressJob = scope.launch {
            TavernApp.instance.container.turnStreams.state(turnId).collectLatest { state ->
                when (state.phase) {
                    TurnPhase.GENERATING -> {
                        val text = state.text.takeLast(60).ifBlank { "正在演绎…" }
                        notify(buildNotification(title, text, ongoing = true))
                    }
                    TurnPhase.TRUNCATED -> {
                        notify(buildNotification(title, "本段已写完，等待续写", ongoing = false))
                        stopSelfSoon()
                    }
                    TurnPhase.FAILED -> {
                        notify(buildNotification(title, state.error ?: "生成失败", ongoing = false))
                        stopSelfSoon()
                    }
                    TurnPhase.COMMITTED, TurnPhase.IDLE -> {
                        if (state.phase == TurnPhase.COMMITTED) {
                            notify(buildNotification(title, "回复已完成", ongoing = false))
                        }
                        stopSelfSoon()
                    }
                    TurnPhase.AWAITING_CONFIRM -> Unit
                }
            }
        }
    }

    private fun stopSelfSoon() {
        scope.launch {
            delay(1_500)
            stopSelf()
        }
    }

    private fun notify(notification: Notification) {
        val manager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        manager.notify(NOTIFICATION_ID, notification)
    }

    private var sessionId: String = ""
    private var branchId: String = ""

    private fun buildNotification(title: String, text: String, ongoing: Boolean): Notification {
        val intent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
            if (sessionId.isNotBlank()) putExtra(EXTRA_SESSION, sessionId)
            if (branchId.isNotBlank()) putExtra(EXTRA_BRANCH, branchId)
        }
        val pending = PendingIntent.getActivity(
            this,
            0,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.ic_menu_edit)
            .setContentTitle(title.ifBlank { getString(R.string.app_name) })
            .setContentText(text)
            .setOngoing(ongoing)
            .setOnlyAlertOnce(true)
            .setContentIntent(pending)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build()
    }

    private fun createChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        if (manager.getNotificationChannel(CHANNEL_ID) != null) return
        manager.createNotificationChannel(
            NotificationChannel(CHANNEL_ID, "生成进度", NotificationManager.IMPORTANCE_LOW).apply {
                description = "回合生成中与完成后的提示"
            },
        )
    }

    override fun onDestroy() {
        scope.cancel()
        super.onDestroy()
    }

    companion object {
        private const val CHANNEL_ID = "tavern_generation"
        private const val NOTIFICATION_ID = 0x7A11
        private const val ACTION_STOP = "dev.tavernagent.mobile.STOP_GENERATION"
        const val EXTRA_SESSION = "sessionId"
        const val EXTRA_BRANCH = "branchId"
        private const val EXTRA_TURN = "turnId"
        private const val EXTRA_TITLE = "title"

        fun start(context: Context, sessionId: String, branchId: String, turnId: String, title: String) {
            val intent = Intent(context, GenerationService::class.java).apply {
                putExtra(EXTRA_SESSION, sessionId)
                putExtra(EXTRA_BRANCH, branchId)
                putExtra(EXTRA_TURN, turnId)
                putExtra(EXTRA_TITLE, title)
            }
            runCatching {
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                    context.startForegroundService(intent)
                } else {
                    context.startService(intent)
                }
            }
        }

        fun stop(context: Context) {
            context.stopService(Intent(context, GenerationService::class.java))
        }
    }
}

/** 终态通知出口：即使没有前台服务在跑，也能在生成结束时提醒（例如应用仍在后台但服务被系统回收前）。 */
class NotificationPresenter(context: Context) {

    private val appContext = context.applicationContext

    fun notifyFinished(state: TurnStreamState, success: Boolean) {
        val manager = appContext.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        val channel = NotificationChannel(
            "tavern_generation",
            "生成进度",
            NotificationManager.IMPORTANCE_DEFAULT,
        )
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O && manager.getNotificationChannel(channel.id) == null) {
            manager.createNotificationChannel(channel)
        }
        val intent = Intent(appContext, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
            if (state.sessionId.isNotBlank()) putExtra(GenerationService.EXTRA_SESSION, state.sessionId)
            if (state.branchId.isNotBlank()) putExtra(GenerationService.EXTRA_BRANCH, state.branchId)
        }
        val pending = PendingIntent.getActivity(
            appContext,
            0,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        val notification = NotificationCompat.Builder(appContext, channel.id)
            .setSmallIcon(android.R.drawable.ic_menu_edit)
            .setContentTitle(state.sessionTitle.ifBlank { "TavernAgent" })
            .setContentText(if (success) "回复已完成" else (state.error ?: "生成未完成"))
            .setAutoCancel(true)
            .setContentIntent(pending)
            .build()
        runCatching { manager.notify(0x7A12, notification) }
    }
}
