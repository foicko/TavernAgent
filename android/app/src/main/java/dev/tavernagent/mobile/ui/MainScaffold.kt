package dev.tavernagent.mobile.ui

import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.ui.unit.dp
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Chat
import androidx.compose.material.icons.filled.Face
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import dev.tavernagent.mobile.ui.RootViewModel
import dev.tavernagent.mobile.ui.components.BottomTab
import dev.tavernagent.mobile.ui.components.TavernBottomBar
import dev.tavernagent.mobile.ui.nav.Routes
import dev.tavernagent.mobile.ui.screens.archive.ArchiveScreen
import dev.tavernagent.mobile.ui.screens.branch.BranchTreeScreen
import dev.tavernagent.mobile.ui.screens.cards.CardsScreen
import dev.tavernagent.mobile.ui.screens.cards.CharacterDetailScreen
import dev.tavernagent.mobile.ui.screens.characters.CharactersScreen
import dev.tavernagent.mobile.ui.screens.chats.ChatsScreen
import dev.tavernagent.mobile.ui.screens.director.DirectorScreen
import dev.tavernagent.mobile.ui.screens.memory.MemoryScreen
import dev.tavernagent.mobile.ui.screens.models.ModelsScreen
import dev.tavernagent.mobile.ui.screens.settings.SettingsScreen
import dev.tavernagent.mobile.ui.screens.story.StoryScreen

/** 三个主入口：聊天（对话列表）、角色（可对话的角色）、设置。 */
private val tabs = listOf(
    BottomTab(Routes.CHATS, "聊天", Icons.AutoMirrored.Filled.Chat),
    BottomTab(Routes.CARDS, "角色", Icons.Filled.Face),
    BottomTab(Routes.SETTINGS, "设置", Icons.Filled.Settings),
)

@Composable
fun MainScaffold(
    onDisconnect: () -> Unit,
    pendingDeepLink: RootViewModel.DeepLinkTarget? = null,
    onConsumeDeepLink: () -> Unit = {},
) {
    val navController = rememberNavController()
    val backStackEntry by navController.currentBackStackEntryAsState()
    val currentRoute = backStackEntry?.destination?.route
    val showBottomBar = tabs.any { it.route == currentRoute }

    LaunchedEffect(pendingDeepLink) {
        pendingDeepLink?.let { target ->
            navController.navigate(Routes.story(target.sessionId, target.branchId)) {
                launchSingleTop = true
            }
            onConsumeDeepLink()
        }
    }

    Scaffold(
        contentWindowInsets = WindowInsets(0, 0, 0, 0),
        bottomBar = {
            if (showBottomBar) {
                TavernBottomBar(
                    tabs = tabs,
                    currentRoute = currentRoute,
                    onSelect = { route ->
                        navController.navigate(route) {
                            popUpTo(navController.graph.findStartDestination().id) { saveState = true }
                            launchSingleTop = true
                            restoreState = true
                        }
                    },
                )
            }
        },
    ) { padding ->
        NavHost(
            navController = navController,
            startDestination = Routes.CHATS,
            modifier = Modifier
                .fillMaxSize()
                .padding(bottom = if (showBottomBar) padding.calculateBottomPadding() else 0.dp),
        ) {
            composable(Routes.CHATS) {
                ChatsScreen(
                    onOpenChat = { sessionId, branchId ->
                        navController.navigate(Routes.story(sessionId, branchId))
                    },
                    onFindCharacters = {
                        navController.navigate(Routes.CARDS) { launchSingleTop = true }
                    },
                )
            }
            composable(Routes.CARDS) {
                CardsScreen(
                    onOpenCharacter = { cardId ->
                        navController.navigate(Routes.characterDetail(cardId))
                    },
                )
            }
            composable(Routes.SETTINGS) {
                SettingsScreen(
                    onOpenModels = { navController.navigate(Routes.MODELS) },
                    onOpenArchive = { navController.navigate(Routes.ARCHIVE) },
                    onDisconnect = onDisconnect,
                )
            }
            composable(Routes.MODELS) {
                ModelsScreen(onBack = { navController.popBackStack() })
            }
            composable(Routes.ARCHIVE) {
                ArchiveScreen(onBack = { navController.popBackStack() })
            }

            composable(
                route = Routes.CHARACTER_DETAIL,
                arguments = listOf(
                    navArgument(Routes.ARG_CARD) { type = NavType.StringType },
                ),
            ) { entry ->
                CharacterDetailScreen(
                    cardId = entry.arguments?.getString(Routes.ARG_CARD).orEmpty(),
                    onBack = { navController.popBackStack() },
                    onOpenChat = { sessionId, branchId ->
                        navController.navigate(Routes.story(sessionId, branchId))
                    },
                )
            }

            composable(
                route = Routes.STORY,
                arguments = listOf(
                    navArgument(Routes.ARG_SESSION) { type = NavType.StringType },
                    navArgument(Routes.ARG_BRANCH) { type = NavType.StringType },
                    navArgument(Routes.ARG_VIEW_NODE) { type = NavType.StringType; defaultValue = Routes.NO_VIEW_NODE },
                ),
            ) { entry ->
                val sessionId = entry.arguments?.getString(Routes.ARG_SESSION).orEmpty()
                val branchId = entry.arguments?.getString(Routes.ARG_BRANCH).orEmpty()
                val viewNodeId = entry.arguments?.getString(Routes.ARG_VIEW_NODE).orEmpty()
                StoryScreen(
                    sessionId = sessionId,
                    branchId = branchId,
                    viewNodeId = viewNodeId,
                    onBack = { navController.popBackStack() },
                    onOpenBranchTree = { sid, bid -> navController.navigate(Routes.branchTree(sid, bid)) },
                    onOpenCharacters = { sid, bid -> navController.navigate(Routes.characters(sid, bid)) },
                    onOpenMemory = { sid, bid -> navController.navigate(Routes.memory(sid, bid)) },
                    onOpenDirector = { sid, bid -> navController.navigate(Routes.director(sid, bid)) },
                    onOpenSettings = { navController.navigate(Routes.MODELS) },
                    onOpenCharacterProfile = { cardId ->
                        if (cardId.isNotBlank()) navController.navigate(Routes.characterDetail(cardId))
                    },
                )
            }

            composable(
                route = Routes.BRANCH_TREE,
                arguments = listOf(
                    navArgument(Routes.ARG_SESSION) { type = NavType.StringType },
                    navArgument(Routes.ARG_BRANCH) { type = NavType.StringType },
                ),
            ) { entry ->
                val sessionId = entry.arguments?.getString(Routes.ARG_SESSION).orEmpty()
                val branchId = entry.arguments?.getString(Routes.ARG_BRANCH).orEmpty()
                BranchTreeScreen(
                    sessionId = sessionId,
                    branchId = branchId,
                    onOpenNode = { sid, bid, nodeId -> navController.navigate(Routes.story(sid, bid, nodeId)) },
                    onBranchChanged = { newBranchId -> navController.navigate(Routes.story(sessionId, newBranchId)) },
                    onBack = { navController.popBackStack() },
                )
            }

            composable(
                route = Routes.CHARACTERS,
                arguments = listOf(
                    navArgument(Routes.ARG_SESSION) { type = NavType.StringType },
                    navArgument(Routes.ARG_BRANCH) { type = NavType.StringType },
                ),
            ) { entry ->
                CharactersScreen(
                    sessionId = entry.arguments?.getString(Routes.ARG_SESSION).orEmpty(),
                    branchId = entry.arguments?.getString(Routes.ARG_BRANCH).orEmpty(),
                    onBack = { navController.popBackStack() },
                )
            }

            composable(
                route = Routes.MEMORY,
                arguments = listOf(
                    navArgument(Routes.ARG_SESSION) { type = NavType.StringType },
                    navArgument(Routes.ARG_BRANCH) { type = NavType.StringType },
                ),
            ) { entry ->
                MemoryScreen(
                    sessionId = entry.arguments?.getString(Routes.ARG_SESSION).orEmpty(),
                    branchId = entry.arguments?.getString(Routes.ARG_BRANCH).orEmpty(),
                    onBack = { navController.popBackStack() },
                )
            }

            composable(
                route = Routes.DIRECTOR,
                arguments = listOf(
                    navArgument(Routes.ARG_SESSION) { type = NavType.StringType },
                    navArgument(Routes.ARG_BRANCH) { type = NavType.StringType },
                ),
            ) { entry ->
                DirectorScreen(
                    sessionId = entry.arguments?.getString(Routes.ARG_SESSION).orEmpty(),
                    branchId = entry.arguments?.getString(Routes.ARG_BRANCH).orEmpty(),
                    onBack = { navController.popBackStack() },
                )
            }
        }
    }
}
