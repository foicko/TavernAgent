package dev.tavernagent.mobile.ui.screens.cards

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.DeleteOutline
import androidx.compose.material.icons.filled.Face
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.tavernagent.mobile.data.remote.dto.CharacterCardEntryDto
import dev.tavernagent.mobile.ui.components.ConfirmDialog
import dev.tavernagent.mobile.ui.components.EmptyState
import dev.tavernagent.mobile.ui.components.ErrorCard
import dev.tavernagent.mobile.ui.components.LoadingBox
import dev.tavernagent.mobile.ui.components.StoryAvatar
import dev.tavernagent.mobile.ui.theme.dimens

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CardsScreen(onOpenCharacter: (cardId: String) -> Unit) {
    val context = LocalContext.current
    val viewModel: CardsViewModel = viewModel(factory = CardsViewModel.factory())
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    var query by remember { mutableStateOf("") }
    var pendingDelete by remember { mutableStateOf<CharacterCardEntryDto?>(null) }

    val importLauncher = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) viewModel.import(context, uri)
    }

    LaunchedEffect(state.toast) {
        state.toast?.let {
            snackbar.showSnackbar(it)
            viewModel.clearToast()
        }
    }

    val keyword = query.trim()
    val filtered = remember(state.cards, keyword) {
        if (keyword.isEmpty()) {
            state.cards
        } else {
            state.cards.filter { card ->
                card.name.contains(keyword, ignoreCase = true) ||
                    card.role?.contains(keyword, ignoreCase = true) == true ||
                    card.shortName?.contains(keyword, ignoreCase = true) == true
            }
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("角色") },
                actions = {
                    IconButton(onClick = viewModel::load) {
                        Icon(Icons.Filled.Refresh, contentDescription = "刷新")
                    }
                    IconButton(onClick = { importLauncher.launch(arrayOf("*/*")) }) {
                        Icon(Icons.Filled.Add, contentDescription = "导入角色")
                    }
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            Column(modifier = Modifier.fillMaxSize()) {
                OutlinedTextField(
                    value = query,
                    onValueChange = { query = it },
                    placeholder = { Text("搜索角色") },
                    leadingIcon = { Icon(Icons.Filled.Search, contentDescription = null) },
                    singleLine = true,
                    shape = RoundedCornerShape(999.dp),
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = dimens.space4, vertical = dimens.space1),
                )
                when {
                    state.loading -> LoadingBox()
                    state.cards.isEmpty() && state.error == null -> EmptyState(
                        title = "还没有角色",
                        hint = "导入 PNG / JSON / charx 角色卡：点右上角 +，从手机文件里选择。",
                        icon = Icons.Filled.Face,
                        actionLabel = "导入角色卡",
                        onAction = { importLauncher.launch(arrayOf("*/*")) },
                    )
                    filtered.isEmpty() -> EmptyState(
                        title = "没有匹配的角色",
                        hint = "换个关键词试试。",
                        icon = Icons.Filled.Search,
                    )
                    else -> LazyColumn(
                        modifier = Modifier.fillMaxSize(),
                        contentPadding = PaddingValues(
                            horizontal = dimens.space4,
                            vertical = dimens.space3,
                        ),
                        verticalArrangement = Arrangement.spacedBy(dimens.space3),
                    ) {
                        state.error?.let { message ->
                            item { ErrorCard(message = message, onRetry = viewModel::load) }
                        }
                        items(filtered, key = { it.cardId }) { card ->
                            CharacterRow(
                                card = card,
                                avatarUrl = state.avatars[card.cardId],
                                chatCount = state.chatCounts[card.cardId] ?: 0,
                                onOpen = { onOpenCharacter(card.cardId) },
                                onDelete = { pendingDelete = card },
                            )
                        }
                    }
                }
            }
            if (state.busy) {
                Box(
                    modifier = Modifier
                        .fillMaxSize()
                        .background(MaterialTheme.colorScheme.scrim.copy(alpha = 0.25f)),
                    contentAlignment = Alignment.Center,
                ) {
                    CircularProgressIndicator()
                }
            }
        }
    }

    pendingDelete?.let { card ->
        ConfirmDialog(
            title = "删除角色",
            text = "将从角色库移除「${card.name}」。已有的对话不受影响。",
            confirmLabel = "删除",
            danger = true,
            onConfirm = {
                viewModel.delete(card.cardId)
                pendingDelete = null
            },
            onDismiss = { pendingDelete = null },
        )
    }
}

@Composable
private fun CharacterRow(
    card: CharacterCardEntryDto,
    avatarUrl: String?,
    chatCount: Int,
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
            verticalAlignment = Alignment.CenterVertically,
        ) {
            StoryAvatar(
                name = card.name,
                avatarUrl = avatarUrl,
                size = 56.dp,
                shape = RoundedCornerShape(16.dp),
            )
            Spacer(Modifier.width(dimens.space3))
            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(
                    text = card.name.ifBlank { "未命名角色" },
                    style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.SemiBold,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                card.role?.takeIf { it.isNotBlank() }?.let { role ->
                    Text(
                        text = role,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                Text(
                    text = if (chatCount > 0) "$chatCount 段对话" else "还没有对话",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.8f),
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
                        text = { Text("删除角色") },
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
