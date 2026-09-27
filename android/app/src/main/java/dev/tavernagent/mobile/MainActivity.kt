package dev.tavernagent.mobile

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.getValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import dev.tavernagent.mobile.data.local.ThemeMode
import dev.tavernagent.mobile.data.local.UserPrefs
import dev.tavernagent.mobile.service.GenerationService
import dev.tavernagent.mobile.ui.RootViewModel
import dev.tavernagent.mobile.ui.TavernRoot
import dev.tavernagent.mobile.ui.theme.TavernAgentTheme

class MainActivity : ComponentActivity() {

    private val rootViewModel: RootViewModel by viewModels { RootViewModel.Factory }

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        handleDeepLinkIntent(intent)
        val container = TavernApp.instance.container
        setContent {
            val prefs by container.prefsStore.prefs.collectAsStateWithLifecycle(initialValue = UserPrefs())
            TavernAgentTheme(themeMode = prefs.themeMode) {
                TavernRoot(rootViewModel = rootViewModel)
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        handleDeepLinkIntent(intent)
    }

    private fun handleDeepLinkIntent(intent: Intent?) {
        val sessionId = intent?.getStringExtra(GenerationService.EXTRA_SESSION)
        val branchId = intent?.getStringExtra(GenerationService.EXTRA_BRANCH)
        if (!sessionId.isNullOrBlank()) {
            rootViewModel.handleDeepLink(sessionId, branchId)
        }
    }
}
