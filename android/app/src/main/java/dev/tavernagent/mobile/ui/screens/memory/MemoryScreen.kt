package dev.tavernagent.mobile.ui.screens.memory

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.tavernagent.mobile.data.remote.dto.MemoryViewDto
import dev.tavernagent.mobile.ui.components.EmptyState
import dev.tavernagent.mobile.ui.components.ErrorCard
import dev.tavernagent.mobile.ui.components.LoadingBox
import dev.tavernagent.mobile.ui.theme.LocalTavernColors
import dev.tavernagent.mobile.ui.theme.dimens

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MemoryScreen(
    sessionId: String,
    branchId: String,
    onBack: () -> Unit,
) {
    val viewModel: MemoryViewModel = viewModel(factory = MemoryViewModel.factory())
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    val colors = LocalTavernColors.current

    LaunchedEffect(sessionId, branchId) { viewModel.open(sessionId, branchId) }
    LaunchedEffect(state.message) {
        state.message?.let {
            snackbar.showSnackbar(it)
            viewModel.clearMessage()
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("记忆库") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回")
                    }
                },
                actions = {
                    IconButton(onClick = viewModel::organize, enabled = !state.busy) {
                        Icon(Icons.Filled.AutoAwesome, contentDescription = "整理记忆")
                    }
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            OutlinedTextField(
                value = state.search,
                onValueChange = viewModel::onSearchChange,
                modifier = Modifier.fillMaxWidth().padding(horizontal = dimens.space4, vertical = dimens.space2),
                placeholder = { Text("搜索记忆内容…") },
                leadingIcon = { Icon(Icons.Filled.Search, contentDescription = null) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
            )
            Row(
                modifier = Modifier.padding(horizontal = dimens.space4, vertical = dimens.space1),
                horizontalArrangement = Arrangement.spacedBy(dimens.space2),
            ) {
                listOf("" to "全部", "observed" to "亲历", "reported" to "转述", "inferred" to "推断", "secret" to "秘密")
                    .forEach { (value, label) ->
                        FilterChip(
                            selected = state.kind == value,
                            onClick = { viewModel.setKind(value) },
                            label = { Text(label) },
                        )
                    }
            }
            state.page?.usage?.let { usage ->
                Text(
                    text = "容量 ${usage.used}/${usage.limit} · 受保护 ${usage.protected} · 可回收 ${usage.reclaimable}",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(horizontal = dimens.space4, vertical = dimens.space1),
                )
            }

            Box(modifier = Modifier.fillMaxSize()) {
                when {
                    state.loading -> LoadingBox()
                    state.page?.memories.isNullOrEmpty() && state.error == null -> EmptyState(
                        title = "还没有记忆",
                        hint = "随着剧情推进，AI 会把认知到的事实沉淀在这里。",
                    )
                    else -> LazyColumn(
                        modifier = Modifier.fillMaxSize(),
                        contentPadding = PaddingValues(dimens.space4),
                        verticalArrangement = Arrangement.spacedBy(dimens.space2),
                    ) {
                        state.error?.let { message ->
                            item { ErrorCard(message = message, onRetry = viewModel::refresh) }
                        }
                        items(state.page?.memories.orEmpty(), key = { it.memoryId }) { memory ->
                            MemoryRowCard(
                                memory = memory,
                                colors = colors,
                                onPin = {
                                    viewModel.togglePin(
                                        memory.toRow(),
                                    )
                                },
                                onHide = {
                                    viewModel.toggleHidden(
                                        memory.toRow(),
                                    )
                                },
                                onRevise = {
                                    viewModel.openEditor(
                                        memory.toRow(),
                                    )
                                },
                            )
                        }
                        if (!state.page?.nextCursor.isNullOrBlank()) {
                            item {
                                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.Center) {
                                    TextButton(onClick = viewModel::loadMore, enabled = !state.loadingMore) {
                                        Text(if (state.loadingMore) "加载中…" else "加载更多")
                                    }
                                }
                            }
                        }
                    }
                }
                if (state.busy) {
                    CircularProgressIndicator(modifier = Modifier.align(Alignment.TopCenter).padding(dimens.space3))
                }
            }
        }
    }

    state.editing?.let { row ->
        ReviseDialog(row = row, onDismiss = viewModel::dismissEditor, onConfirm = { viewModel.revise(row, it) })
    }
}

@Composable
private fun MemoryRowCard(
    memory: MemoryViewDto,
    colors: dev.tavernagent.mobile.ui.theme.TavernColors,
    onPin: () -> Unit,
    onHide: () -> Unit,
    onRevise: () -> Unit,
) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = if (memory.hidden) MaterialTheme.colorScheme.surfaceContainerLow
            else MaterialTheme.colorScheme.surfaceContainerHigh,
        ),
    ) {
        Column(modifier = Modifier.padding(dimens.space3), verticalArrangement = Arrangement.spacedBy(dimens.space2)) {
            Row(horizontalArrangement = Arrangement.spacedBy(dimens.space2), verticalAlignment = Alignment.CenterVertically) {
                KindPill(memory.kind, colors)
                if (memory.pinned) Pill("置顶", MaterialTheme.colorScheme.primary)
                if (memory.hidden) Pill("已隐藏", MaterialTheme.colorScheme.outline)
                if (!memory.effective) Pill("已被覆盖", MaterialTheme.colorScheme.outline)
                if (memory.protected) Pill("受保护", MaterialTheme.colorScheme.tertiary)
            }
            Text(
                text = memory.content,
                style = MaterialTheme.typography.bodyMedium,
                color = if (memory.hidden) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurface,
            )
            Row(
                horizontalArrangement = Arrangement.spacedBy(dimens.space2),
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text(
                    text = buildString {
                        if (memory.confidence > 0) append("置信 ${(memory.confidence * 100).toInt()}% ")
                        if (memory.createdTurn > 0) append("· 第 ${memory.createdTurn} 轮")
                    },
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.weight(1f),
                )
                TextButton(onClick = onRevise) { Text("修订") }
                TextButton(onClick = onPin) { Text(if (memory.pinned) "取消置顶" else "置顶") }
                TextButton(onClick = onHide) { Text(if (memory.hidden) "恢复" else "隐藏") }
            }
        }
    }
}

@Composable
private fun KindPill(kind: String, colors: dev.tavernagent.mobile.ui.theme.TavernColors) {
    val (label, color) = when (kind) {
        "observed" -> "亲历" to colors.memoryEpisodic
        "reported" -> "转述" to colors.memoryInference
        "inferred" -> "推断" to colors.memoryInference
        "secret" -> "秘密" to colors.memorySecret
        else -> kind to MaterialTheme.colorScheme.outline
    }
    Pill(label, color)
}

@Composable
private fun Pill(text: String, color: Color) {
    Box(
        modifier = Modifier
            .background(color.copy(alpha = 0.15f), RoundedCornerShape(999.dp))
            .padding(horizontal = dimens.space2, vertical = 2.dp),
    ) {
        Text(text, style = MaterialTheme.typography.labelSmall, color = color)
    }
}

@Composable
private fun ReviseDialog(
    row: MemoryViewModel.MemoryRow,
    onDismiss: () -> Unit,
    onConfirm: (String) -> Unit,
) {
    var content by remember { mutableStateOf(row.content) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("修订记忆") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(dimens.space2)) {
                Text(
                    "修订采用分支级覆盖：原记录保留，新记录只影响当前分支。",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                OutlinedTextField(
                    value = content,
                    onValueChange = { content = it },
                    modifier = Modifier.fillMaxWidth(),
                    maxLines = 6,
                )
            }
        },
        confirmButton = {
            Button(onClick = { onConfirm(content.trim()) }, enabled = content.isNotBlank()) { Text("保存修订") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } },
    )
}
