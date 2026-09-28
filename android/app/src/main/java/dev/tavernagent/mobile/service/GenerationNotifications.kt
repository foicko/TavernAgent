package dev.tavernagent.mobile.service

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.os.Build

/**
 * 生成通知渠道的唯一创建点（由 `TavernApp.onCreate` 调用一次）。
 *
 * 此前前台服务与 [NotificationPresenter] 各建一次同名渠道、importance 不同（LOW vs DEFAULT），
 * 谁先创建谁生效，可能让"进行中"通知发声/横幅。现在统一为 IMPORTANCE_LOW。
 */
object GenerationNotifications {
    const val CHANNEL_ID = "tavern_generation"

    fun ensureChannel(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        if (manager.getNotificationChannel(CHANNEL_ID) != null) return
        manager.createNotificationChannel(
            NotificationChannel(CHANNEL_ID, "生成进度", NotificationManager.IMPORTANCE_LOW).apply {
                description = "回合生成中与完成后的提示"
            },
        )
    }
}
