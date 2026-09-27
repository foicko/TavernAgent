package dev.tavernagent.mobile.ui.screens.chats

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Chat
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.DeleteOutline
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.tavernagent.mobile.ui.components.ConfirmDialog
import dev.tavernagent.mobile.ui.components.EmptyState
import dev.tavernagent.mobile.ui.components.ErrorCard
import dev.tavernagent.mobile.ui.components.LoadingBox
import dev.tavernagent.mobile.ui.components.StoryAvatar
import dev.tavernagent.mobile.ui.components.relativeTime
import dev.tavernagent.mobile.ui.theme.dimens

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ChatsScreen(
    onOpenChat: (sessionId: String, branchId: String) -> Unit,
    onFindCharacters: () -> Unit,
) {
    val viewModel: ChatsViewModel = viewModel(factory = ChatsViewModel.factory)
    val state by viewModel.state.collectAsStateWithLifecycle()
    var pendingDelete by remember { mutableStateOf<ChatsViewModel.ChatItem?>(null) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("聊天") },
                actions = {
                    IconButton(onClick = viewModel::load) {
                        Icon(Icons.Filled.Refresh, contentDescription = "刷新")
                    }
                },
            )
        },
        floatingActionButton = {
            ExtendedFloatingActionButton(
                onClick = onFindCharacters,
                icon = { Icon(Icons.Filled.Add, contentDescription = null) },
                text = { Text("新对话") },
            )
        },
    ) { padding ->
        Surface(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.loading -> LoadingBox()
                state.error != null && state.items.isEmpty() -> ErrorCard(
                    message = state.error ?: "加载失败",
                    modifier = Modifier.padding(dimens.space4),
                    onRetry = viewModel::load,
                )
                state.items.isEmpty() -> EmptyState(
                    title = "还没有对话",
                    hint = "挑一个角色，开始你们的第一次对话。",
                    icon = Icons.AutoMirrored.Filled.Chat,
                    actionLabel = "选择角色",
                    onAction = onFindCharacters,
                )
                else -> LazyColumn(
                    modifier = Modifier.fillMaxSize(),
                    contentPadding = PaddingValues(
                        start = dimens.space4,
                        end = dimens.space4,
                        top = dimens.space3,
                        bottom = 96.dp,
                    ),
                    verticalArrangement = Arrangement.spacedBy(dimens.space2),
                ) {
                    state.error?.let { message ->
                        item { ErrorCard(message = message, onRetry = viewModel::load, onSecondary = viewModel::clearError, secondaryLabel = "知道了") }
                    }
                    items(state.items, key = { it.sessionId }) { chat ->
                        ChatRow(
                            chat = chat,
                            isLast = chat.sessionId == state.lastSessionId,
                            onOpen = { onOpenChat(chat.sessionId, "_") },
                            onDelete = { pendingDelete = chat },
                        )
                    }
                }
            }
        }
    }

    pendingDelete?.let { chat ->
        ConfirmDialog(
            title = "删除对话",
            text = "将永久删除与「${chat.name}」的这段对话及其全部记录，无法恢复。",
            confirmLabel = "删除",
            danger = true,
            onConfirm = {
                viewModel.delete(chat.sessionId)
                pendingDelete = null
            },
            onDismiss = { pendingDelete = null },
        )
    }
}

@Composable
private fun ChatRow(
    chat: ChatsViewModel.ChatItem,
    isLast: Boolean,
    onOpen: () -> Unit,
    onDelete: () -> Unit,
) {
    var menuOpen by remember { mutableStateOf(false) }

    Surface(
        color = MaterialTheme.colorScheme.surfaceContainerHigh,
        shape = RoundedCornerShape(dimens.space3),
        modifier = Modifier.fillMaxWidth().clickable(onClick = onOpen),
    ) {
        Row(
            modifier = Modifier.padding(start = dimens.space3, top = dimens.space3, bottom = dimens.space3, end = dimens.space1),
            verticalAlignment = Alignment.Top,
        ) {
            StoryAvatar(
                name = chat.name,
                avatarUrl = chat.avatarUrl,
                size = 52.dp,
                shape = RoundedCornerShape(16.dp),
            )
            Spacer(Modifier.width(dimens.space3))
            Column(modifier = Modifier.weight(1f)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        text = chat.name,
                        style = MaterialTheme.typography.titleMedium,
                        fontWeight = FontWeight.SemiBold,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f, fill = false),
                    )
                    if (isLast) {
                        Spacer(Modifier.width(dimens.space2))
                        Text(
                            text = "继续",
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.primary,
                        )
                    }
                    Spacer(Modifier.weight(1f))
                    Text(
                        text = relativeTime(chat.updatedAt),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Spacer(Modifier.height(2.dp))
                Text(
                    text = chat.preview?.takeIf { it.isNotBlank() } ?: "还没有消息",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            Box {
                IconButton(onClick = { menuOpen = true }) {
                    Icon(
                        Icons.Filled.MoreVert,
                        contentDescription = "更多",
                        tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                    DropdownMenuItem(
                        text = { Text("删除对话") },
                        leadingIcon = { Icon(Icons.Filled.DeleteOutline, contentDescription = null) },
                        onClick = {
                            menuOpen = false
                            onDelete()
                        },
                    )
                }
            }
        }
    }
}
