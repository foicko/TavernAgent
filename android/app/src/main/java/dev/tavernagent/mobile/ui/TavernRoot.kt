package dev.tavernagent.mobile.ui

import android.Manifest
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.tavernagent.mobile.ui.screens.connect.ConnectScreen
import dev.tavernagent.mobile.ui.theme.dimens

@Composable
fun TavernRoot(rootViewModel: RootViewModel = viewModel(factory = RootViewModel.Factory)) {
    val state by rootViewModel.state.collectAsStateWithLifecycle()
    val pendingDeepLink by rootViewModel.pendingDeepLink.collectAsStateWithLifecycle()

    // 生成完成通知（Android 13+ 需要运行时权限）。
    val notificationPermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { }
    androidx.compose.runtime.LaunchedEffect(Unit) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
        }
    }

    Surface(modifier = Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
        AnimatedContent(
            targetState = state,
            transitionSpec = { fadeIn() togetherWith fadeOut() },
            label = "root",
        ) { current ->
            when (current) {
                is RootViewModel.State.Restoring -> RestoringScreen()
                is RootViewModel.State.NeedsConnect -> ConnectScreen(onConnected = rootViewModel::onConnected)
                is RootViewModel.State.Ready -> MainScaffold(
                    onDisconnect = rootViewModel::disconnect,
                    pendingDeepLink = pendingDeepLink,
                    onConsumeDeepLink = rootViewModel::consumeDeepLink,
                )
            }
        }
    }
}

@Composable
private fun RestoringScreen() {
    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(dimens.space3),
        ) {
            Text(text = "TavernAgent", style = MaterialTheme.typography.headlineMedium)
            CircularProgressIndicator()
            Text(
                text = "正在恢复连接…",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}
