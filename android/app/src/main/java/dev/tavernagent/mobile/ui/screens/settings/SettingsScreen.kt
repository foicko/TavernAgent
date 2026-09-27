package dev.tavernagent.mobile.ui.screens.settings

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.Archive
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.GraphicEq
import androidx.compose.material.icons.filled.Image
import androidx.compose.material.icons.filled.Link
import androidx.compose.material.icons.filled.LinkOff
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.RecordVoiceOver
import androidx.compose.material.icons.filled.SmartToy
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Slider
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import coil.compose.AsyncImage
import dev.tavernagent.mobile.BuildConfig
import dev.tavernagent.mobile.data.local.ThemeMode
import dev.tavernagent.mobile.playback.TtsEngine
import dev.tavernagent.mobile.ui.components.ConfirmDialog
import dev.tavernagent.mobile.ui.theme.dimens

/**
 * 设置页：分组卡片 + 行式操作。
 *
 * 结构：连接 → 外观 → 模型与备份 → 朗读 → 插画 → 关于。
 * 视觉规则：卡片用发丝描边而不是阴影；行距、字重与次级文字统一走 theme 令牌。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    onOpenModels: () -> Unit,
    onOpenArchive: () -> Unit,
    onDisconnect: () -> Unit,
) {
    val context = LocalContext.current
    val viewModel: SettingsViewModel = viewModel(factory = SettingsViewModel.factory())
    val state by viewModel.state.collectAsStateWithLifecycle()
    val snackbar = remember { SnackbarHostState() }
    var imageUrl by remember { mutableStateOf<String?>(null) }
    var imagePrompt by remember { mutableStateOf("酒馆里的温暖灯火，人物特写，柔光") }
    var confirmDisconnect by remember { mutableStateOf(false) }
    var rate by remember(state.ttsRate) { mutableFloatStateOf(state.ttsRate) }
    var pitch by remember(state.ttsPitch) { mutableFloatStateOf(state.ttsPitch) }

    LaunchedEffect(state.message) {
        state.message?.let {
            snackbar.showSnackbar(it)
            viewModel.clearMessage()
        }
    }

    Scaffold(
        topBar = { TopAppBar(title = { Text("设置") }) },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        LazyColumn(
            modifier = Modifier.fillMaxSize().padding(padding),
            contentPadding = PaddingValues(horizontal = dimens.space4, vertical = dimens.space3),
            verticalArrangement = Arrangement.spacedBy(dimens.space5),
        ) {
            item {
                SettingsGroup(title = "连接") {
                    ConnectionRow(baseUrl = state.baseUrl)
                    RowDivider()
                    ActionRow(
                        icon = Icons.Filled.LinkOff,
                        title = "断开并重新配对",
                        hint = "清除本机保存的地址与配对凭据",
                        tint = MaterialTheme.colorScheme.error,
                        onClick = { confirmDisconnect = true },
                    )
                }
            }

            item {
                SettingsGroup(title = "外观") {
                    Column(modifier = Modifier.padding(horizontal = dimens.space4, vertical = dimens.space3)) {
                        Text(
                            text = "主题",
                            style = MaterialTheme.typography.bodyLarge,
                            fontWeight = FontWeight.Medium,
                        )
                        Spacer(Modifier.heightIn(min = dimens.space2))
                        SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
                            val options = listOf(
                                ThemeMode.SYSTEM to "跟随系统",
                                ThemeMode.LIGHT to "浅色",
                                ThemeMode.DARK to "深色",
                                ThemeMode.VIOLET to "幻夜紫",
                            )
                            options.forEachIndexed { index, (mode, label) ->
                                SegmentedButton(
                                    selected = state.themeMode == mode,
                                    onClick = { viewModel.setTheme(mode) },
                                    shape = SegmentedButtonDefaults.itemShape(index = index, count = options.size),
                                ) { Text(label, maxLines = 1) }
                            }
                        }
                    }
                }
            }

            item {
                SettingsGroup(title = "模型与备份") {
                    ActionRow(
                        icon = Icons.Filled.SmartToy,
                        title = "模型设置",
                        hint = "对话 / 导演 / 记忆三槽位",
                        onClick = onOpenModels,
                    )
                    RowDivider()
                    ActionRow(
                        icon = Icons.Filled.Archive,
                        title = "对话备份",
                        hint = "导出 / 导入 .tavernpack",
                        onClick = onOpenArchive,
                    )
                }
            }

            item {
                SettingsGroup(title = "朗读") {
                    SwitchRow(
                        icon = Icons.Filled.GraphicEq,
                        title = "自动朗读新回复",
                        checked = state.autoRead,
                        onCheckedChange = viewModel::setAutoRead,
                    )
                    RowDivider()
                    SliderRow(
                        label = "语速",
                        value = rate,
                        onValueChange = { rate = it },
                        onFinished = { viewModel.setTts(rate, pitch, state.autoRead) },
                    )
                    RowDivider()
                    SliderRow(
                        label = "音调",
                        value = pitch,
                        onValueChange = { pitch = it },
                        onFinished = { viewModel.setTts(rate, pitch, state.autoRead) },
                    )
                    if (state.voices.isNotEmpty()) {
                        RowDivider()
                        Column(modifier = Modifier.padding(horizontal = dimens.space4, vertical = dimens.space3)) {
                            Text(
                                text = "后端音色",
                                style = MaterialTheme.typography.labelMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                            LazyRow(
                                horizontalArrangement = Arrangement.spacedBy(dimens.space2),
                                contentPadding = PaddingValues(vertical = dimens.space1),
                            ) {
                                items(state.voices.take(12), key = { it.id }) { voice ->
                                    FilterChip(
                                        selected = state.selectedVoice == voice.id,
                                        onClick = { viewModel.selectVoice(voice.id) },
                                        label = { Text(voice.name.take(10)) },
                                    )
                                }
                            }
                        }
                    }
                    RowDivider()
                    Row(
                        modifier = Modifier.padding(horizontal = dimens.space4, vertical = dimens.space3),
                        horizontalArrangement = Arrangement.spacedBy(dimens.space2),
                    ) {
                        Button(onClick = {
                            viewModel.setTts(rate, pitch, state.autoRead)
                            TtsEngine.configure(rate, pitch)
                            viewModel.testTts { text -> TtsEngine.speak(context, text) }
                        }) {
                            Icon(Icons.Filled.PlayArrow, contentDescription = null, modifier = Modifier.size(18.dp))
                            Spacer(Modifier.width(dimens.space1))
                            Text("本机试听")
                        }
                        OutlinedButton(onClick = { viewModel.testBackendTts { } }) {
                            Icon(Icons.Filled.RecordVoiceOver, contentDescription = null, modifier = Modifier.size(18.dp))
                            Spacer(Modifier.width(dimens.space1))
                            Text("测试后端合成")
                        }
                    }
                    Caption("聊天页的「朗读」默认使用本机 TTS；后端合成需要服务端配置语音引擎。")
                }
            }

            item {
                SettingsGroup(title = "插画") {
                    Column(
                        modifier = Modifier.padding(dimens.space4),
                        verticalArrangement = Arrangement.spacedBy(dimens.space3),
                    ) {
                        OutlinedTextField(
                            value = imagePrompt,
                            onValueChange = { imagePrompt = it },
                            label = { Text("画面描述") },
                            modifier = Modifier.fillMaxWidth(),
                            minLines = 2,
                            maxLines = 3,
                        )
                        Button(onClick = { viewModel.generateTestImage(imagePrompt) { url -> imageUrl = url } }) {
                            Icon(Icons.Filled.AutoAwesome, contentDescription = null, modifier = Modifier.size(18.dp))
                            Spacer(Modifier.width(dimens.space1))
                            Text("生成插画")
                        }
                        imageUrl?.let { url ->
                            AsyncImage(
                                model = url,
                                contentDescription = null,
                                contentScale = ContentScale.Fit,
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .heightIn(max = 260.dp)
                                    .clip(RoundedCornerShape(12.dp)),
                            )
                            Text(
                                text = url,
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                        }
                        Text(
                            text = "生图凭据由服务端提供（/api/v1/images/generate）；本机不保存生图密钥。",
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }

            item {
                Column(
                    modifier = Modifier.fillMaxWidth().padding(horizontal = dimens.space1, vertical = dimens.space2),
                    verticalArrangement = Arrangement.spacedBy(dimens.space1),
                ) {
                    Text(
                        text = "TavernAgent 移动端",
                        style = MaterialTheme.typography.titleSmall,
                        fontWeight = FontWeight.SemiBold,
                    )
                    Text(
                        text = "版本 ${BuildConfig.VERSION_NAME} · 本地优先的 AI 角色扮演客户端",
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Text(
                        text = "所有对话、状态与记忆均由你自己的服务端提供。",
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.8f),
                    )
                }
            }
        }
    }

    if (confirmDisconnect) {
        ConfirmDialog(
            title = "断开连接",
            text = "将清除本机保存的服务地址与配对凭据，需要重新配对才能继续使用。",
            confirmLabel = "断开",
            danger = true,
            onConfirm = {
                confirmDisconnect = false
                onDisconnect()
            },
            onDismiss = { confirmDisconnect = false },
        )
    }
}

@Composable
private fun SettingsGroup(title: String, content: @Composable ColumnScope.() -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(dimens.space2)) {
        Text(
            text = title,
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(start = dimens.space1),
        )
        Surface(
            color = MaterialTheme.colorScheme.surface,
            shape = RoundedCornerShape(16.dp),
            border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.8f)),
            modifier = Modifier.fillMaxWidth(),
        ) {
            Column(content = content)
        }
    }
}

@Composable
private fun ConnectionRow(baseUrl: String) {
    val connected = baseUrl.isNotBlank()
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = dimens.space4, vertical = dimens.space3),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            modifier = Modifier
                .size(9.dp)
                .clip(CircleShape)
                .background(if (connected) MaterialTheme.colorScheme.secondary else MaterialTheme.colorScheme.error),
        )
        Spacer(Modifier.width(dimens.space3))
        Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(1.dp)) {
            Text(
                text = if (connected) "已连接" else "未连接",
                style = MaterialTheme.typography.bodyLarge,
                fontWeight = FontWeight.Medium,
            )
            Text(
                text = if (connected) {
                    baseUrl.removePrefix("http://").removePrefix("https://").trimEnd('/')
                } else {
                    "回到「连接」页完成配对"
                },
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        if (connected) {
            Icon(
                Icons.Filled.Link,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.5f),
                modifier = Modifier.size(18.dp),
            )
        }
    }
}

@Composable
private fun ActionRow(
    icon: ImageVector,
    title: String,
    hint: String? = null,
    tint: Color = MaterialTheme.colorScheme.onSurface,
    onClick: (() -> Unit)? = null,
) {
    val isDanger = tint == MaterialTheme.colorScheme.error
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .then(if (onClick != null) Modifier.clickable(onClick = onClick) else Modifier)
            .padding(horizontal = dimens.space4, vertical = dimens.space3),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            imageVector = icon,
            contentDescription = null,
            tint = if (isDanger) tint else MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(20.dp),
        )
        Column(
            modifier = Modifier.weight(1f).padding(start = dimens.space3),
            verticalArrangement = Arrangement.spacedBy(1.dp),
        ) {
            Text(
                text = title,
                style = MaterialTheme.typography.bodyLarge,
                color = tint,
                fontWeight = FontWeight.Medium,
            )
            hint?.let {
                Text(
                    text = it,
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
        if (onClick != null && !isDanger) {
            Icon(
                Icons.AutoMirrored.Filled.KeyboardArrowRight,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.6f),
                modifier = Modifier.size(18.dp),
            )
        }
    }
}

@Composable
private fun SwitchRow(
    icon: ImageVector,
    title: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable { onCheckedChange(!checked) }
            .padding(horizontal = dimens.space4, vertical = dimens.space2),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            imageVector = icon,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(20.dp),
        )
        Text(
            text = title,
            style = MaterialTheme.typography.bodyLarge,
            modifier = Modifier.weight(1f).padding(start = dimens.space3),
        )
        Switch(checked = checked, onCheckedChange = onCheckedChange)
    }
}

@Composable
private fun SliderRow(
    label: String,
    value: Float,
    onValueChange: (Float) -> Unit,
    onFinished: () -> Unit,
) {
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = dimens.space4, vertical = dimens.space2)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                text = label,
                style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier.weight(1f),
            )
            Text(
                text = "%.2f".format(value),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Slider(
            value = value,
            onValueChange = onValueChange,
            onValueChangeFinished = onFinished,
            valueRange = 0.5f..2.0f,
            modifier = Modifier.fillMaxWidth(),
        )
    }
}

@Composable
private fun RowDivider() {
    HorizontalDivider(
        modifier = Modifier.padding(horizontal = dimens.space4),
        thickness = 1.dp,
        color = MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.6f),
    )
}

@Composable
private fun Caption(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.labelSmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(horizontal = dimens.space4, vertical = dimens.space3),
    )
}
