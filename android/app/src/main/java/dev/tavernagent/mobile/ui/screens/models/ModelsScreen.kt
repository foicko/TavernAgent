package dev.tavernagent.mobile.ui.screens.models

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Switch
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
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.tavernagent.mobile.data.remote.dto.ModelInstanceDto
import dev.tavernagent.mobile.data.remote.dto.SlotBindingDto
import dev.tavernagent.mobile.ui.components.ErrorCard
import dev.tavernagent.mobile.ui.components.LoadingBox
import dev.tavernagent.mobile.ui.components.SectionHeader
import dev.tavernagent.mobile.ui.theme.dimens

private val SLOT_LABELS = mapOf(
    "primary" to "对话生成",
    "assist" to "导演协商",
    "reflection" to "后台记忆",
)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ModelsScreen(onBack: () -> Unit) {
    val viewModel: ModelsViewModel = viewModel(factory = ModelsViewModel.factory())
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }

    LaunchedEffect(state.message) {
        state.message?.let {
            snackbar.showSnackbar(it)
            viewModel.clearMessage()
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("模型设置") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回")
                    }
                },
            )
        },
        floatingActionButton = {
            FloatingActionButton(onClick = viewModel::startCreate) {
                Icon(Icons.Filled.Add, contentDescription = "新建模型连接")
            }
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.loading -> LoadingBox()
                else -> LazyColumn(
                    modifier = Modifier.fillMaxSize(),
                    contentPadding = androidx.compose.foundation.layout.PaddingValues(
                        start = dimens.space4,
                        end = dimens.space4,
                        top = dimens.space3,
                        bottom = 96.dp,
                    ),
                    verticalArrangement = Arrangement.spacedBy(dimens.space3),
                ) {
                    state.error?.let { message ->
                        item { ErrorCard(message = message, onRetry = viewModel::load) }
                    }
                    item { SectionHeader("用途指派") }
                    items(listOf("primary", "assist", "reflection")) { slot ->
                        SlotCard(
                            slot = slot,
                            binding = state.slots.firstOrNull { it.slot == slot },
                            models = state.models,
                            busy = state.busy,
                            onSave = { enabled, modelId -> viewModel.setSlot(slot, enabled, modelId) },
                            onProbe = { viewModel.probeSlot(slot, SLOT_LABELS[slot] ?: slot) },
                        )
                    }
                    item { SectionHeader("模型连接") }
                    if (state.models.isEmpty()) {
                        item {
                            Text(
                                "还没有模型连接。点右下角 + 新建（如 DeepSeek / OpenAI / Ollama / Anthropic）。",
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                    items(state.models, key = { it.id }) { model ->
                        ModelRow(
                            model = model,
                            probeResult = state.probeResults[model.id],
                            onEdit = { viewModel.startEdit(model) },
                            onProbe = { viewModel.probeModel(model) },
                            onDelete = { viewModel.delete(model.id) },
                        )
                    }
                }
            }
        }
    }

    state.editing?.let { editing ->
        ModelEditDialog(
            initial = editing,
            busy = state.busy,
            onDismiss = viewModel::dismissEditor,
            onSave = viewModel::save,
        )
    }
}

@Composable
private fun SlotCard(
    slot: String,
    binding: SlotBindingDto?,
    models: List<ModelInstanceDto>,
    busy: Boolean,
    onSave: (Boolean, String) -> Unit,
    onProbe: () -> Unit,
) {
    var enabled by remember(binding?.enabled) { mutableStateOf(binding?.enabled ?: false) }
    var selected by remember(binding?.modelId) { mutableStateOf(binding?.modelId.orEmpty()) }
    var menuOpen by remember { mutableStateOf(false) }

    Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh)) {
        Column(modifier = Modifier.padding(dimens.space3), verticalArrangement = Arrangement.spacedBy(dimens.space2)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    SLOT_LABELS[slot] ?: slot,
                    style = MaterialTheme.typography.titleSmall,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.weight(1f),
                )
                Switch(
                    checked = enabled,
                    onCheckedChange = {
                        enabled = it
                        onSave(it, selected)
                    },
                    enabled = !busy,
                )
            }
            Box {
                OutlinedButton(onClick = { menuOpen = true }, enabled = !busy || true, modifier = Modifier.fillMaxWidth()) {
                    Text(
                        text = models.firstOrNull { it.id == selected }?.let { "${it.name}（${it.model ?: it.kind}）" }
                            ?: "选择模型连接…",
                    )
                }
                DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                    models.forEach { model ->
                        DropdownMenuItem(
                            text = { Text("${model.name}（${model.model ?: model.kind}）") },
                            onClick = {
                                menuOpen = false
                                selected = model.id
                                if (enabled) onSave(enabled, model.id)
                            },
                        )
                    }
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(dimens.space2), verticalAlignment = Alignment.CenterVertically) {
                TextButton(onClick = onProbe, enabled = !busy && enabled) { Text("测试连通性") }
                binding?.let {
                    Text(
                        text = it.resolvedModelId?.takeIf { id -> id.isNotBlank() }?.let { id -> "生效：$id" } ?: "",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

@Composable
private fun ModelRow(
    model: ModelInstanceDto,
    probeResult: String?,
    onEdit: () -> Unit,
    onProbe: () -> Unit,
    onDelete: () -> Unit,
) {
    Card(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onEdit),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh),
    ) {
        Column(modifier = Modifier.padding(dimens.space3), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    model.name.ifBlank { "未命名连接" },
                    style = MaterialTheme.typography.titleSmall,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.weight(1f),
                )
                if (model.hasApiKey) {
                    Text("已存密钥", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.secondary)
                }
                IconButton(onClick = onDelete) {
                    Icon(Icons.Filled.Delete, contentDescription = "删除", tint = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            Text(
                listOfNotNull(
                    model.kind,
                    model.model?.takeIf { it.isNotBlank() },
                    model.baseUrl?.takeIf { it.isNotBlank() },
                    model.contextWindow.takeIf { it > 0 }?.let { "上下文 $it" },
                    model.maxTokens.takeIf { it > 0 }?.let { "输出 $it" },
                ).joinToString(" · "),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Row(horizontalArrangement = Arrangement.spacedBy(dimens.space2), verticalAlignment = Alignment.CenterVertically) {
                TextButton(onClick = onProbe) { Text("测试") }
                probeResult?.let {
                    Text(it, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
    }
}

@Composable
private fun ModelEditDialog(
    initial: ModelInstanceDto,
    busy: Boolean,
    onDismiss: () -> Unit,
    onSave: (ModelInstanceDto) -> Unit,
) {
    var name by remember { mutableStateOf(initial.name) }
    var kind by remember { mutableStateOf(initial.kind.ifBlank { "openai-chat" }) }
    var baseUrl by remember { mutableStateOf(initial.baseUrl.orEmpty()) }
    var model by remember { mutableStateOf(initial.model.orEmpty()) }
    var apiKey by remember { mutableStateOf("") }
    var contextWindow by remember { mutableStateOf(initial.contextWindow.takeIf { it > 0 }?.toString() ?: "128000") }
    var maxTokens by remember { mutableStateOf(initial.maxTokens.takeIf { it > 0 }?.toString() ?: "8192") }
    var kindMenu by remember { mutableStateOf(false) }

    androidx.compose.material3.AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(if (initial.id.isBlank()) "新建模型连接" else "编辑模型连接") },
        text = {
            Column(
                modifier = Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(dimens.space3),
            ) {
                OutlinedTextField(value = name, onValueChange = { name = it }, label = { Text("名称") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                Box {
                    OutlinedButton(onClick = { kindMenu = true }, modifier = Modifier.fillMaxWidth()) {
                        Text(
                            when (kind) {
                                "openai-responses" -> "OpenAI Responses"
                                "anthropic" -> "Anthropic Messages"
                                else -> "OpenAI Chat Completions"
                            },
                        )
                    }
                    DropdownMenu(expanded = kindMenu, onDismissRequest = { kindMenu = false }) {
                        listOf(
                            "openai-chat" to "OpenAI Chat Completions",
                            "openai-responses" to "OpenAI Responses",
                            "anthropic" to "Anthropic Messages",
                        ).forEach { (value, label) ->
                            DropdownMenuItem(text = { Text(label) }, onClick = { kindMenu = false; kind = value })
                        }
                    }
                }
                OutlinedTextField(
                    value = baseUrl,
                    onValueChange = { baseUrl = it },
                    label = { Text("Base URL（到版本根，如 https://api.deepseek.com/v1）") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(value = model, onValueChange = { model = it }, label = { Text("模型名") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                OutlinedTextField(
                    value = apiKey,
                    onValueChange = { apiKey = it },
                    label = { Text(if (initial.hasApiKey) "API Key（留空表示保留原密钥）" else "API Key") },
                    singleLine = true,
                    keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(keyboardType = KeyboardType.Password),
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    value = contextWindow,
                    onValueChange = { contextWindow = it.filter(Char::isDigit) },
                    label = { Text("上下文窗口（tokens）") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    value = maxTokens,
                    onValueChange = { maxTokens = it.filter(Char::isDigit) },
                    label = { Text("输出上限（tokens）") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                Text(
                    "推理模型的思考也占用输出预算：DeepSeek / Gemini Thinking 等请给到 8192 以上；留空时服务端只预留 2048，容易被思考吃满、出现「只有思考没有回答」。",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
              Text(
                    "Key 只保存在服务端；本机不保存模型密钥。",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        },
        confirmButton = {
            Button(
                onClick = {
                    onSave(
                        initial.copy(
                            name = name.trim(),
                            kind = kind,
                            baseUrl = baseUrl.trim().takeIf { it.isNotBlank() },
                            model = model.trim().takeIf { it.isNotBlank() },
                            apiKey = apiKey.trim().takeIf { it.isNotBlank() },
                            contextWindow = contextWindow.toIntOrNull() ?: 0,
                            maxTokens = maxTokens.toIntOrNull() ?: 0,
                        ),
                    )
                },
                enabled = !busy && name.isNotBlank(),
            ) { Text("保存") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } },
    )
}
