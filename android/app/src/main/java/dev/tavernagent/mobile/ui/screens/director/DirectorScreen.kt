package dev.tavernagent.mobile.ui.screens.director

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.tavernagent.mobile.data.remote.dto.DirectorBeatDto
import dev.tavernagent.mobile.ui.components.ErrorCard
import dev.tavernagent.mobile.ui.components.LoadingBox
import dev.tavernagent.mobile.ui.components.SectionHeader
import dev.tavernagent.mobile.ui.theme.dimens

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DirectorScreen(
    sessionId: String,
    branchId: String,
    onBack: () -> Unit,
) {
    val viewModel: DirectorViewModel = viewModel(factory = DirectorViewModel.factory())
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }

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
                title = { Text("导演模式") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回")
                    }
                },
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.loading -> LoadingBox()
                state.error != null && state.directorView == null -> ErrorCard(
                    message = state.error ?: "导演服务不可用",
                    modifier = Modifier.padding(dimens.space4),
                    onRetry = viewModel::refresh,
                )
                else -> Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .verticalScroll(rememberScrollState())
                        .imePadding()
                        .padding(dimens.space4),
                    verticalArrangement = Arrangement.spacedBy(dimens.space3),
                ) {
                    state.error?.let { message ->
                        ErrorCard(message = message, onRetry = viewModel::refresh)
                    }

                    val directorState = state.directorView?.state
                    Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh)) {
                        Column(modifier = Modifier.padding(dimens.space3), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                            Text(
                                directorState?.plan?.title?.ifBlank { "未命名大纲" } ?: "尚未编排大纲",
                                style = MaterialTheme.typography.titleMedium,
                                fontWeight = FontWeight.SemiBold,
                            )
                            Text(
                                text = when (directorState?.status) {
                                    "active" -> "推进中 · 已完成 ${directorState.progress.count { it.value.status == "completed" }} / ${directorState.plan.beats.size}"
                                    "paused" -> "已暂停"
                                    "completed" -> "已完成"
                                    null -> "未启用"
                                    else -> directorState.status
                                },
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                            directorState?.warning?.takeIf { it.isNotBlank() }?.let {
                                Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
                            }
                        }
                    }

                    SectionHeader("三阶段命令")
                    Row(horizontalArrangement = Arrangement.spacedBy(dimens.space2), modifier = Modifier.fillMaxWidth()) {
                        FilledTonalButton(onClick = { viewModel.command("activate") }, enabled = !state.busy) { Text("启用/应用") }
                        OutlinedButton(onClick = { viewModel.command(if (directorState?.status == "paused") "resume" else "pause") }) {
                            Text(if (directorState?.status == "paused") "继续" else "暂停")
                        }
                        OutlinedButton(onClick = { viewModel.command("complete") }) { Text("完成当前") }
                    }
                    Row(horizontalArrangement = Arrangement.spacedBy(dimens.space2), modifier = Modifier.fillMaxWidth()) {
                        OutlinedButton(onClick = { viewModel.command("skip") }) { Text("跳过当前") }
                        OutlinedButton(onClick = { viewModel.command("rewind") }) { Text("回退一步") }
                    }

                    SectionHeader("大纲草稿")
                    OutlinedTextField(
                        value = state.draftTitle,
                        onValueChange = viewModel::editTitle,
                        label = { Text("标题") },
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true,
                    )
                    OutlinedTextField(
                        value = state.draftGuidance,
                        onValueChange = viewModel::editGuidance,
                        label = { Text("总体指导") },
                        modifier = Modifier.fillMaxWidth(),
                        maxLines = 4,
                    )
                    state.draftBeats.forEachIndexed { index, beat ->
                        Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow)) {
                            Column(modifier = Modifier.padding(dimens.space3), verticalArrangement = Arrangement.spacedBy(dimens.space2)) {
                                Row(verticalAlignment = Alignment.CenterVertically) {
                                    Text("节拍 ${index + 1}", style = MaterialTheme.typography.labelLarge, modifier = Modifier.weight(1f))
                                    IconButton(onClick = { viewModel.removeBeat(index) }) {
                                        Icon(Icons.Filled.Delete, contentDescription = "删除节拍")
                                    }
                                }
                                OutlinedTextField(
                                    value = beat.title,
                                    onValueChange = { viewModel.editBeat(index, beat.copy(title = it)) },
                                    label = { Text("标题") },
                                    modifier = Modifier.fillMaxWidth(),
                                    singleLine = true,
                                )
                                OutlinedTextField(
                                    value = beat.instruction,
                                    onValueChange = { viewModel.editBeat(index, beat.copy(instruction = it)) },
                                    label = { Text("指令") },
                                    modifier = Modifier.fillMaxWidth(),
                                    maxLines = 3,
                                )
                                OutlinedTextField(
                                    value = beat.completionCriteria,
                                    onValueChange = { viewModel.editBeat(index, beat.copy(completionCriteria = it)) },
                                    label = { Text("完成判定") },
                                    modifier = Modifier.fillMaxWidth(),
                                    maxLines = 2,
                                )
                            }
                        }
                    }
                    Row(horizontalArrangement = Arrangement.spacedBy(dimens.space2)) {
                        OutlinedButton(onClick = viewModel::addBeat) {
                            Icon(Icons.Filled.Add, contentDescription = null)
                            Text("添加节拍")
                        }
                        Button(onClick = viewModel::saveDraft, enabled = !state.busy && state.draftDirty) {
                            Text("保存草稿")
                        }
                    }

                    SectionHeader("与 AI 讨论")
                    state.directorView?.requests?.take(6)?.forEach { request ->
                        Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow)) {
                            Column(modifier = Modifier.padding(dimens.space3), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                                Text("你：${request.text}", style = MaterialTheme.typography.bodyMedium)
                                request.reply?.takeIf { it.isNotBlank() }?.let {
                                    Text("导演：$it", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                } ?: Text(
                                    text = "状态：${request.status}",
                                    style = MaterialTheme.typography.labelSmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                        }
                    }
                    state.activeRequest?.takeIf { it.status == "generating" }?.let {
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(dimens.space2)) {
                            CircularProgressIndicator(modifier = Modifier.padding(dimens.space1).fillMaxWidth(0.04f))
                            Text("导演正在思考…", style = MaterialTheme.typography.labelMedium, modifier = Modifier.weight(1f))
                            OutlinedButton(onClick = viewModel::cancelRequest) { Text("停止") }
                        }
                    }
                    state.activeRequest?.candidate?.let {
                        OutlinedButton(onClick = viewModel::applyCandidate, enabled = !state.busy) { Text("把候选填入草稿") }
                    }
                    OutlinedTextField(
                        value = state.discussion,
                        onValueChange = viewModel::onDiscussionChange,
                        label = { Text("和导演讨论走向…") },
                        modifier = Modifier.fillMaxWidth(),
                        maxLines = 4,
                        enabled = !state.busy && !state.polling,
                    )
                    Row(horizontalArrangement = Arrangement.spacedBy(dimens.space2)) {
                        Button(onClick = viewModel::discuss, enabled = !state.busy && state.discussion.isNotBlank()) { Text("发起讨论") }
                        OutlinedButton(onClick = viewModel::refresh) { Text("刷新") }
                    }
                }
            }
        }
    }
}
