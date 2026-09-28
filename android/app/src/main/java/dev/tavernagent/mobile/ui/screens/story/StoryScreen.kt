package dev.tavernagent.mobile.ui.screens.story

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.sizeIn
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.MenuBook
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.AccountTree
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Groups
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.Memory
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Psychology
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SmallFloatingActionButton
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.tavernagent.mobile.data.remote.dto.SessionViewDto
import dev.tavernagent.mobile.domain.TurnPhase
import dev.tavernagent.mobile.playback.readAloud
import dev.tavernagent.mobile.service.GenerationService
import dev.tavernagent.mobile.ui.components.ConfirmDialog
import dev.tavernagent.mobile.ui.components.ErrorCard
import dev.tavernagent.mobile.ui.components.ImageLightbox
import dev.tavernagent.mobile.ui.components.LoadingBox
import dev.tavernagent.mobile.ui.components.StoryAvatar
import dev.tavernagent.mobile.ui.theme.LocalTavernColors
import dev.tavernagent.mobile.ui.theme.dimens

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun StoryScreen(
    sessionId: String,
    branchId: String,
    viewNodeId: String,
    onBack: () -> Unit,
    onOpenBranchTree: (sessionId: String, branchId: String) -> Unit,
    onOpenCharacters: (sessionId: String, branchId: String) -> Unit,
    onOpenMemory: (sessionId: String, branchId: String) -> Unit,
    onOpenDirector: (sessionId: String, branchId: String) -> Unit,
    onOpenSettings: () -> Unit,
    onOpenCharacterProfile: (cardId: String) -> Unit = {},
) {
    val viewModel: StoryViewModel = viewModel(factory = StoryViewModel.factory())
    val state by viewModel.state.collectAsStateWithLifecycle()
    val colors = LocalTavernColors.current
    val context = LocalContext.current
    val snackbar = remember { SnackbarHostState() }
    val listState = rememberLazyListState()
    var menuOpen by remember { mutableStateOf(false) }
    var regenerateFor by remember { mutableStateOf<String?>(null) }
    var forkFor by remember { mutableStateOf<String?>(null) }
    var showBranches by remember { mutableStateOf(false) }
    var previewImageUrl by remember { mutableStateOf<String?>(null) }
    val worldSheetState = rememberModalBottomSheetState()
    var showWorldStateSheet by remember { mutableStateOf(false) }

    LaunchedEffect(state.toast) {
        state.toast?.let {
            snackbar.showSnackbar(it)
            viewModel.clearToast()
        }
    }

    LaunchedEffect(sessionId, branchId, viewNodeId) {
        if (state.sessionId != sessionId || state.branchId != branchId || state.viewingNodeId != viewNodeId.takeIf { it != "_" }) {
            viewModel.open(sessionId, branchId, viewNodeId)
        }
    }

    // 生成期间启动前台服务：切后台/锁屏也能继续接收流，完成后通知。
    LaunchedEffect(state.currentTurnId, state.phase) {
        val turnId = state.currentTurnId
        if (turnId != null && state.phase != TurnPhase.IDLE && state.phase != TurnPhase.FAILED) {
            GenerationService.start(context, sessionId, state.branchId, turnId, state.title)
        } else {
            GenerationService.stop(context)
        }
    }

    // 新增回合或流式生成时平滑滚动吸底（reverseLayout 下 index 0 为最新内容）
    LaunchedEffect(state.messages.size, state.draftBlocks.size, state.phase, state.pendingEcho?.text) {
        if (state.phase == TurnPhase.GENERATING || state.messages.isNotEmpty()) {
            listState.animateScrollToItem(0)
        }
    }

    // 自动朗读新回复：每个节点只读一次，回看历史或从别的页面返回不会重读。
    var lastAutoReadNode by rememberSaveable { mutableStateOf("") }
    LaunchedEffect(state.phase, state.messages.size) {
        val latest = state.messages.lastOrNull()
        if (state.autoRead && state.phase == TurnPhase.COMMITTED && latest != null && latest.nodeId != lastAutoReadNode) {
            lastAutoReadNode = latest.nodeId
            val text = latest.blocks.joinToString("\n") { it.text }.trim()
            if (text.isNotBlank()) readAloud(context, text)
        }
    }

    Scaffold(
        contentWindowInsets = WindowInsets(0, 0, 0, 0),
        topBar = {
            TopAppBar(
                windowInsets = WindowInsets.statusBars,
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回")
                    }
                },
                title = {
                    // 聊天页标题就是「角色」：点头像/名字进入角色资料（继续或新开对话）。
                    val characterId = state.view?.characterId.orEmpty()
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        modifier = Modifier
                            .sizeIn(minHeight = 48.dp)
                            .clickable(
                                enabled = characterId.isNotBlank(),
                                role = Role.Button,
                                onClick = { onOpenCharacterProfile(characterId) },
                            ),
                    ) {
                        StoryAvatar(
                            name = state.resolvedCharacterName,
                            avatarUrl = state.characterAvatarUrl,
                            size = 34.dp,
                            shape = CircleShape,
                        )
                        Spacer(Modifier.width(dimens.space2))
                        Column {
                            Text(
                                text = state.resolvedCharacterName.ifBlank { state.title.ifBlank { "对话" } },
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                            val branch = state.view?.branch?.name
                                ?.takeIf { it.isNotBlank() && !it.equals("main", ignoreCase = true) }
                                ?.let { "分支 · $it" }
                            Text(
                                text = listOfNotNull(state.title.takeIf { it.isNotBlank() }, branch).joinToString(" · ").ifBlank { "对话" },
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                        }
                    }
                },
                actions = {
                    IconButton(onClick = { onOpenBranchTree(state.sessionId, state.branchId) }) {
                        Icon(Icons.Filled.AccountTree, contentDescription = "分支与回溯")
                    }
                    IconButton(onClick = { showWorldStateSheet = true }) {
                        Icon(Icons.Filled.Groups, contentDescription = "角色与状态")
                    }
                    IconButton(onClick = { onOpenMemory(state.sessionId, state.branchId) }) {
                        Icon(Icons.Filled.Memory, contentDescription = "记忆库")
                    }
                    IconButton(onClick = { menuOpen = true }) {
                        Icon(Icons.Filled.MoreVert, contentDescription = "更多")
                    }
                    DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                        DropdownMenuItem(
                            text = { Text("导演模式") },
                            leadingIcon = { Icon(Icons.Filled.Psychology, contentDescription = null) },
                            onClick = {
                                menuOpen = false
                                onOpenDirector(state.sessionId, state.branchId)
                            },
                        )
                        DropdownMenuItem(
                            text = { Text("刷新视图") },
                            leadingIcon = { Icon(Icons.Filled.Refresh, contentDescription = null) },
                            onClick = {
                                menuOpen = false
                                viewModel.refresh()
                            },
                        )
                        DropdownMenuItem(
                            text = { Text("切换到分支…") },
                            leadingIcon = { Icon(Icons.Filled.AccountTree, contentDescription = null) },
                            onClick = {
                                menuOpen = false
                                showBranches = true
                            },
                        )
                        DropdownMenuItem(
                            text = { Text("模型设置") },
                            leadingIcon = { Icon(Icons.AutoMirrored.Filled.MenuBook, contentDescription = null) },
                            onClick = {
                                menuOpen = false
                                onOpenSettings()
                            },
                        )
                    }
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
        bottomBar = {
            StoryComposer(
                state = state,
                onInputChange = viewModel::onInputChange,
                onNoteChange = viewModel::onNoteChange,
                onToggleNote = viewModel::toggleNote,
                onOptionsMode = viewModel::setOptionsMode,
                onSend = viewModel::send,
                onCancel = viewModel::cancel,
                onContinue = viewModel::continueTurn,
                onRetry = viewModel::retry,
                onBackToHead = viewModel::backToHead,
                onChooseOption = viewModel::chooseOption,
                onSwitchModel = viewModel::switchPrimaryModel,
                onSetReasoningEffort = viewModel::setReasoningEffort,
                onOpenSettings = onOpenSettings,
            )
        },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.loading -> LoadingBox(label = "正在打开对话…")
                state.view == null -> ErrorCard(
                    message = state.error ?: "无法加载对话",
                    modifier = Modifier.padding(dimens.space4),
                    onRetry = { viewModel.refresh() },
                )
                else -> {
                    val resolver = remember(state.view?.characterId, state.view?.sessionId, state.cardAssets, state.baseUrl) {
                        imageResolver(state)
                    }
                    val reversed = state.messages.asReversed()
                    LazyColumn(
                        state = listState,
                        reverseLayout = true,
                        modifier = Modifier.fillMaxSize(),
                        contentPadding = PaddingValues(
                            horizontal = dimens.space4,
                            vertical = dimens.space4,
                        ),
                    ) {
                        // 最新内容在底部：reverseLayout 下 index 0 即最新。
                        item(key = "stream") {
                            Column(
                                modifier = Modifier.fillMaxWidth().padding(bottom = dimens.space3),
                                verticalArrangement = Arrangement.spacedBy(dimens.space3),
                            ) {
                                // 玩家消息乐观回显：发送即显示，提交后由服务端节点接管。
                                state.pendingEcho?.let { echo ->
                                    PlayerEchoBubble(
                                        text = echo.text,
                                        note = echo.note,
                                        playerName = state.resolvedPlayerName,
                                        avatarUrl = state.playerAvatarUrl,
                                    )
                                }
                                if (state.phase.isBusy || state.draftBlocks.isNotEmpty() || state.thinking.isNotBlank()) {
                                    if (state.thinking.isNotBlank()) {
                                        ThinkingBlock(
                                            thinking = state.thinking,
                                            seconds = state.thinkingSeconds,
                                            streaming = state.phase == TurnPhase.GENERATING,
                                        )
                                    }
                                    if (state.draftBlocks.isNotEmpty()) {
                                        StreamingDraft(
                                            blocks = state.draftBlocks,
                                            colors = colors,
                                            resolveImage = resolver,
                                            characterName = state.resolvedCharacterName,
                                            characterAvatarUrl = state.characterAvatarUrl,
                                            onImageClick = { url -> previewImageUrl = url },
                                        )
                                    } else if (state.phase == TurnPhase.GENERATING) {
                                        Row(
                                            horizontalArrangement = Arrangement.spacedBy(dimens.space2),
                                            verticalAlignment = Alignment.CenterVertically,
                                        ) {
                                            CircularProgressIndicator(modifier = Modifier.size(14.dp), strokeWidth = 2.dp)
                                            Text("正在演绎…", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                        }
                                    }
                                }
                                state.error?.let { message ->
                                    ErrorCard(
                                        message = message,
                                        retryable = state.errorRetryable,
                                        onRetry = viewModel::retry,
                                        onSecondary = onOpenSettings,
                                        secondaryLabel = "模型设置",
                                    )
                                }
                                if (state.phase == TurnPhase.TRUNCATED) {
                                    Surface(
                                        color = MaterialTheme.colorScheme.tertiaryContainer,
                                        shape = RoundedCornerShape(12.dp),
                                        modifier = Modifier.fillMaxWidth(),
                                    ) {
                                        Column(
                                            modifier = Modifier.padding(dimens.space3),
                                            verticalArrangement = Arrangement.spacedBy(2.dp),
                                        ) {
                                            Text(
                                                "本轮被输出上限截断",
                                                style = MaterialTheme.typography.labelLarge,
                                                color = MaterialTheme.colorScheme.onTertiaryContainer,
                                            )
                                            Text(
                                                "推理模型的思考也会占用输出预算；点「续写」可以接着写，或到「模型设置」把输出上限调到 8192 以上。",
                                                style = MaterialTheme.typography.bodySmall,
                                                color = MaterialTheme.colorScheme.onTertiaryContainer.copy(alpha = 0.9f),
                                            )
                                        }
                                    }
                                }
                            }
                        }

                        reversed.forEachIndexed { index, message ->
                            val isLatest = index == 0
                            item(key = message.nodeId) {
                                Column(modifier = Modifier.fillMaxWidth().padding(bottom = dimens.space3)) {
                                    StoryMessageItem(
                                        message = message,
                                        colors = colors,
                                        resolveImage = resolver,
                                        showOptions = isLatest && !state.busy && !state.viewingHistory,
                                        busy = state.busy,
                                        onChooseOption = viewModel::chooseOption,
                                        onReadAloud = { text ->
                                            readAloud(context, text)
                                        },
                                        onRegenerate = { regenerateFor = message.nodeId },
                                        onFork = { forkFor = message.nodeId },
                                        onViewAt = { viewModel.viewAt(message.nodeId) },
                                        characterName = state.resolvedCharacterName,
                                        characterAvatarUrl = state.characterAvatarUrl,
                                        playerName = state.resolvedPlayerName,
                                        playerAvatarUrl = state.playerAvatarUrl,
                                        onImageClick = { url -> previewImageUrl = url },
                                        onQuote = { text -> viewModel.onInputChange(text) },
                                        onOpenCharacterDetails = { showWorldStateSheet = true },
                                    )
                                }
                            }
                        }

                        if (state.view?.hasMore == true) {
                            item(key = "more") {
                                Row(
                                    modifier = Modifier.fillMaxWidth().padding(vertical = dimens.space3),
                                    horizontalArrangement = Arrangement.Center,
                                ) {
                                    OutlinedButton(onClick = viewModel::loadMoreHistory, enabled = !state.historyLoading) {
                                        Text(if (state.historyLoading) "加载中…" else "加载更早的消息")
                                    }
                                }
                            }
                        }
                    }

                    if (state.viewingHistory) {
                        Surface(
                            modifier = Modifier.align(Alignment.TopCenter).padding(top = dimens.space2),
                            color = MaterialTheme.colorScheme.tertiaryContainer,
                            shape = RoundedCornerShape(999.dp),
                        ) {
                            Row(
                                modifier = Modifier.padding(horizontal = dimens.space3, vertical = dimens.space1),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(dimens.space2),
                            ) {
                                Text("这是历史记录（只读）", style = MaterialTheme.typography.labelSmall)
                                TextButton(onClick = viewModel::backToHead) { Text("回到最新") }
                            }
                        }
                    }
                }
            }
        }
    }

    regenerateFor?.let { nodeId ->
        RegenerateDialog(
            onDismiss = { regenerateFor = null },
            onConfirm = { recheck, note ->
                regenerateFor = null
                viewModel.regenerate(nodeId, recheck, note)
            },
        )
    }

    forkFor?.let { nodeId ->
        ConfirmDialog(
            title = "从此分叉",
            text = "将从此处开辟新分支，原分支不受影响。",
            confirmLabel = "开辟分支",
            onConfirm = { viewModel.fork(nodeId) },
            onDismiss = { forkFor = null },
        )
    }

    if (showBranches) {
        BranchPickerDialog(
            view = state.view,
            onDismiss = { showBranches = false },
            onPick = { branchId ->
                showBranches = false
                viewModel.selectBranchFromView(branchId)
            },
        )
    }

    ImageLightbox(
        imageUrl = previewImageUrl,
        onDismiss = { previewImageUrl = null },
    )

    if (showWorldStateSheet) {
        WorldStateSheet(
            view = state.view,
            world = state.worldState,
            sheetState = worldSheetState,
            onDismiss = { showWorldStateSheet = false },
            onOpenFullDetail = {
                showWorldStateSheet = false
                onOpenCharacters(state.sessionId, state.branchId)
            },
        )
    }
}



@Composable
private fun RegenerateDialog(onDismiss: () -> Unit, onConfirm: (recheck: Boolean, note: String?) -> Unit) {
    var recheck by remember { mutableStateOf(false) }
    var note by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("重新生成这条回复") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(dimens.space3)) {
                Text("会在上一条消息之后生成新的候选回复，原回复不会被改写。", style = MaterialTheme.typography.bodyMedium)
                OutlinedTextField(
                    value = note,
                    onValueChange = { note = it },
                    label = { Text("引导要求（可选）") },
                    modifier = Modifier.fillMaxWidth(),
                )
                Row(verticalAlignment = Alignment.CenterVertically) {
                    FilterChip(selected = recheck, onClick = { recheck = !recheck }, label = { Text("重新掷骰") })
                }
            }
        },
        confirmButton = {
            Button(onClick = { onConfirm(recheck, note.takeIf { it.isNotBlank() }) }) { Text("生成候选") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } },
    )
}

@Composable
private fun BranchPickerDialog(view: SessionViewDto?, onDismiss: () -> Unit, onPick: (String) -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("切换到分支") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(dimens.space2)) {
                view?.branches?.forEach { branch ->
                    val isCurrent = branch.branchId == view.branch.branchId
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clip(RoundedCornerShape(dimens.space2))
                            .background(
                                if (isCurrent) MaterialTheme.colorScheme.primaryContainer
                                else MaterialTheme.colorScheme.surfaceContainerHigh,
                            )
                            .padding(dimens.space3),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Column {
                            Text(branch.name.ifBlank { branch.branchId }, style = MaterialTheme.typography.bodyMedium)
                            Text(
                                if (isCurrent) "当前分支" else "第 ${branch.version} 版",
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                        if (!isCurrent) {
                            TextButton(onClick = { onPick(branch.branchId) }) { Text("切换") }
                        }
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text("关闭") } },
    )
}

private fun imageResolver(state: StoryUiState): (String) -> String? {
    val base = viewBaseUrl(state)
    // 资源表建一次索引：同一张卡的几十个标记不再逐次线性扫描（Anis 卡有 700+ 条资源）。
    val index = dev.tavernagent.mobile.domain.CardAssetResolver.index(state.cardAssets)
    return { raw ->
        val key = raw.trim()
        // 资源表尚未就绪时返回 null：渲染端按「加载中」处理，等表到达后自动重解析。
        // 绝不按裸键拼 URL——charx 卡的 ref 与名字不同名，那种请求必然 404。
        if (key.isEmpty()) null else dev.tavernagent.mobile.domain.CardAssetResolver.resolveIndexed(index, key, base)
    }
}

private fun viewBaseUrl(state: StoryUiState): String = state.baseUrl
