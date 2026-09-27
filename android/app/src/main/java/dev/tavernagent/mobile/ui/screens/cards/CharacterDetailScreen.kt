package dev.tavernagent.mobile.ui.screens.cards

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.DeleteOutline
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
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
import dev.tavernagent.mobile.data.remote.dto.OpeningVariantDto
import dev.tavernagent.mobile.ui.components.ConfirmDialog
import dev.tavernagent.mobile.ui.components.ErrorCard
import dev.tavernagent.mobile.ui.components.LoadingBox
import dev.tavernagent.mobile.ui.components.StoryAvatar
import dev.tavernagent.mobile.ui.components.relativeTime
import dev.tavernagent.mobile.ui.theme.dimens

/**
 * 角色资料页：头像是角色、名字是角色、下面直接是「继续对话 / 新开对话」。
 *
 * 人设、性格、场景与开场白全部来自服务端卡预览；这里不编辑、不推断。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CharacterDetailScreen(
    cardId: String,
    onBack: () -> Unit,
    onOpenChat: (sessionId: String, branchId: String) -> Unit,
) {
    val viewModel: CharacterDetailViewModel = viewModel(
        key = "character-$cardId",
        factory = CharacterDetailViewModel.factory(cardId),
    )
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    var menuOpen by remember { mutableStateOf(false) }
    var showStart by remember { mutableStateOf(false) }
    var pendingDelete by remember { mutableStateOf(false) }

    LaunchedEffect(state.toast) {
        state.toast?.let {
            snackbar.showSnackbar(it)
            viewModel.clearToast()
        }
    }
    LaunchedEffect(state.deleted) {
        if (state.deleted) onBack()
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("角色资料") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回")
                    }
                },
                actions = {
                    IconButton(onClick = { menuOpen = true }) {
                        Icon(Icons.Filled.MoreVert, contentDescription = "更多")
                    }
                    DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                        DropdownMenuItem(
                            text = { Text("删除角色") },
                            leadingIcon = { Icon(Icons.Filled.DeleteOutline, contentDescription = null) },
                            onClick = {
                                menuOpen = false
                                pendingDelete = true
                            },
                        )
                    }
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        when {
            state.loading -> LoadingBox(modifier = Modifier.padding(padding), label = "正在读取角色…")
            state.error != null && state.name.isBlank() -> ErrorCard(
                message = state.error ?: "读取角色失败",
                modifier = Modifier.padding(padding).padding(dimens.space4),
                onRetry = viewModel::load,
            )
            else -> Column(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(padding)
                    .verticalScroll(rememberScrollState())
                    .padding(horizontal = dimens.space4, vertical = dimens.space3),
                verticalArrangement = Arrangement.spacedBy(dimens.space4),
            ) {
                Header(state)

                Button(
                    onClick = { showStart = true },
                    enabled = !state.busy,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Icon(Icons.Filled.Add, contentDescription = null)
                    Spacer(Modifier.width(dimens.space2))
                    Text("开始新对话")
                }

                if (state.chats.isNotEmpty()) {
                    ChatListSection(
                        characterName = state.name,
                        chats = state.chats,
                        onOpenChat = { sessionId -> onOpenChat(sessionId, "_") },
                    )
                }

                state.description?.takeIf { it.isNotBlank() }?.let { Section("人设", it) }
                state.personality?.takeIf { it.isNotBlank() }?.let { Section("性格", it) }
                state.scenario?.takeIf { it.isNotBlank() }?.let { Section("场景", it) }

                if (state.openings.isNotEmpty()) {
                    Section("开场白") {
                        state.openings.forEach { variant ->
                            Surface(
                                color = MaterialTheme.colorScheme.surfaceContainerLowest,
                                shape = RoundedCornerShape(12.dp),
                                modifier = Modifier.fillMaxWidth().padding(top = dimens.space2),
                            ) {
                                Column(
                                    modifier = Modifier.padding(dimens.space3),
                                    verticalArrangement = Arrangement.spacedBy(2.dp),
                                ) {
                                    Text(
                                        text = variant.title.ifBlank { "开场" },
                                        style = MaterialTheme.typography.labelLarge,
                                    )
                                    Text(
                                        text = variant.text,
                                        style = MaterialTheme.typography.bodySmall,
                                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                                        maxLines = 4,
                                        overflow = TextOverflow.Ellipsis,
                                    )
                                }
                            }
                        }
                    }
                }

                state.format?.takeIf { it.isNotBlank() }?.let { format ->
                    Text(
                        text = "格式：$format",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }

                if (state.warnings.isNotEmpty()) {
                    Text(
                        text = state.warnings.joinToString("；"),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                    )
                }

                state.error?.takeIf { state.name.isNotBlank() }?.let { message ->
                    ErrorCard(message = message, onRetry = viewModel::load)
                }
            }
        }
    }

    if (showStart) {
        StartChatDialog(
            characterName = state.name,
            openings = state.openings,
            busy = state.busy,
            onDismiss = { showStart = false },
            onConfirm = { title, playerName, playerRole, opening ->
                showStart = false
                viewModel.startChat(title, playerName, playerRole, opening, onOpenChat)
            },
        )
    }

    if (pendingDelete) {
        ConfirmDialog(
            title = "删除角色",
            text = "将从角色库移除「${state.name}」。已有的对话不受影响。",
            confirmLabel = "删除",
            danger = true,
            onConfirm = {
                pendingDelete = false
                viewModel.deleteCard()
            },
            onDismiss = { pendingDelete = false },
        )
    }
}

@Composable
private fun Header(state: CharacterDetailViewModel.State) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        StoryAvatar(
            name = state.name,
            avatarUrl = state.avatarUrl,
            size = 88.dp,
            shape = RoundedCornerShape(24.dp),
        )
        Spacer(Modifier.width(dimens.space4))
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(
                text = state.name,
                style = MaterialTheme.typography.headlineSmall,
                fontWeight = FontWeight.Bold,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
            state.role?.takeIf { it.isNotBlank() }?.let { role ->
                Text(
                    text = role,
                    style = MaterialTheme.typography.titleSmall,
                    color = MaterialTheme.colorScheme.primary,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            if (state.tags.isNotEmpty()) {
                Row(horizontalArrangement = Arrangement.spacedBy(dimens.space1)) {
                    state.tags.take(3).forEach { tag ->
                        FilterChip(selected = false, onClick = {}, label = { Text(tag) })
                    }
                }
            }
        }
    }
}

@Composable
private fun ChatListSection(
    characterName: String,
    chats: List<CharacterDetailViewModel.ChatRef>,
    onOpenChat: (sessionId: String) -> Unit,
) {
    Column(verticalArrangement = Arrangement.spacedBy(dimens.space2)) {
        Text(
            text = "与$characterName 的对话",
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        chats.take(6).forEach { chat ->
            Surface(
                color = MaterialTheme.colorScheme.surfaceContainerHigh,
                shape = RoundedCornerShape(12.dp),
                modifier = Modifier.fillMaxWidth().clickable { onOpenChat(chat.sessionId) },
            ) {
                Row(
                    modifier = Modifier.padding(horizontal = dimens.space3, vertical = dimens.space3),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Column(modifier = Modifier.weight(1f)) {
                        Text(
                            text = chat.title.ifBlank { characterName },
                            style = MaterialTheme.typography.bodyMedium,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                        Text(
                            text = relativeTime(chat.updatedAt),
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    Icon(
                        Icons.AutoMirrored.Filled.KeyboardArrowRight,
                        contentDescription = null,
                        tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

@Composable
private fun Section(title: String, text: String? = null, content: (@Composable () -> Unit)? = null) {
    Column(verticalArrangement = Arrangement.spacedBy(dimens.space1)) {
        Text(
            text = title,
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        text?.let { Text(text = it, style = MaterialTheme.typography.bodyMedium) }
        content?.invoke()
    }
}

@Composable
private fun StartChatDialog(
    characterName: String,
    openings: List<OpeningVariantDto>,
    busy: Boolean,
    onDismiss: () -> Unit,
    onConfirm: (title: String, playerName: String, playerRole: String, opening: OpeningVariantDto?) -> Unit,
) {
    var title by remember { mutableStateOf(characterName) }
    var playerName by remember { mutableStateOf("旅人") }
    var playerRole by remember { mutableStateOf("") }
    var opening by remember { mutableStateOf(openings.firstOrNull()) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("开始新对话") },
        text = {
            Column(
                modifier = Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(dimens.space3),
            ) {
                OutlinedTextField(
                    value = title,
                    onValueChange = { title = it },
                    label = { Text("对话名称") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    value = playerName,
                    onValueChange = { playerName = it },
                    label = { Text("你希望角色怎么称呼你") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    value = playerRole,
                    onValueChange = { playerRole = it },
                    label = { Text("你的身份（可选）") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                if (openings.size > 1) {
                    Text("开场白", style = MaterialTheme.typography.labelLarge)
                    openings.forEach { variant ->
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable { opening = variant },
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            RadioButton(selected = opening?.variantId == variant.variantId, onClick = { opening = variant })
                            Column(modifier = Modifier.weight(1f)) {
                                Text(variant.title.ifBlank { "开场" }, style = MaterialTheme.typography.bodyMedium)
                                Text(
                                    variant.text.take(80),
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    maxLines = 2,
                                    overflow = TextOverflow.Ellipsis,
                                )
                            }
                        }
                    }
                }
            }
        },
        confirmButton = {
            Button(
                onClick = { onConfirm(title, playerName, playerRole, opening) },
                enabled = !busy && title.isNotBlank(),
            ) { Text("开始") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } },
    )
}
