package dev.tavernagent.mobile.ui.screens.story

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.EditNote
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Psychology
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.SmartToy
import androidx.compose.material.icons.filled.Stop
import androidx.compose.material.icons.filled.Tune
import androidx.compose.material3.AssistChip
import androidx.compose.material3.AssistChipDefaults
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.tavernagent.mobile.data.remote.dto.ModelInstanceDto
import dev.tavernagent.mobile.data.remote.dto.OptionDto
import dev.tavernagent.mobile.domain.TurnPhase
import dev.tavernagent.mobile.ui.theme.LocalTavernColors
import dev.tavernagent.mobile.ui.theme.dimens

/**
 * 现代化输入控制台（Bottom Control Bar）：
 * - 快速抉择与选项胶囊栏（Action Chips Carousel）
 * - 沉浸式多行自适应输入框
 * - 交互状态按钮组（发送 / 中断 / 续写 / 重试 / 导演注记）
 * - 键盘平滑防遮挡（精确管理 navigationBarsPadding 与 imePadding）
 */
@Composable
fun StoryComposer(
    state: StoryUiState,
    onInputChange: (String) -> Unit,
    onNoteChange: (String) -> Unit,
    onToggleNote: () -> Unit,
    onOptionsMode: (String) -> Unit,
    onSend: () -> Unit,
    onCancel: () -> Unit,
    onContinue: () -> Unit,
    onRetry: () -> Unit,
    onBackToHead: () -> Unit,
    onChooseOption: (OptionDto) -> Unit = {},
    onSwitchModel: (String) -> Unit = {},
    onSetReasoningEffort: (String) -> Unit = {},
    onOpenSettings: () -> Unit = {},
    modifier: Modifier = Modifier,
) {
    val colors = LocalTavernColors.current

    Surface(
        color = MaterialTheme.colorScheme.surfaceContainerLow,
        tonalElevation = 3.dp,
        modifier = modifier
            .fillMaxWidth()
            .navigationBarsPadding()
            .imePadding(),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = dimens.space3, vertical = dimens.space2),
            verticalArrangement = Arrangement.spacedBy(dimens.space2),
        ) {
            // 只读历史快照提示
            if (state.viewingHistory) {
                ReadOnlyHistoryBar(onBackToHead = onBackToHead)
                return@Column
            }

            // 1. 快速抉择与选项胶囊横向滚动栏
            ActionChipsBar(
                state = state,
                onChooseOption = onChooseOption,
                onToggleNote = onToggleNote,
                onOptionsMode = onOptionsMode,
                onSwitchModel = onSwitchModel,
                onSetReasoningEffort = onSetReasoningEffort,
                onOpenSettings = onOpenSettings,
            )

            // 2. 可折叠展开的导演/玩家注记卡片
            AnimatedVisibility(
                visible = state.noteVisible,
                enter = expandVertically() + fadeIn(),
                exit = shrinkVertically() + fadeOut(),
            ) {
                NoteInputCard(
                    note = state.inputNote,
                    onNoteChange = onNoteChange,
                    onClose = onToggleNote,
                )
            }

            // 3. 核心多行输入栏与交互控制按钮组
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.Bottom,
                horizontalArrangement = Arrangement.spacedBy(dimens.space2),
            ) {
                // 自适应输入框
                OutlinedTextField(
                    value = state.inputText,
                    onValueChange = onInputChange,
                    modifier = Modifier
                        .weight(1f)
                        .heightIn(min = 48.dp, max = 150.dp),
                    placeholder = {
                        Text(
                            text = if (state.busy) "正在演绎生成中…" else "输入你的行动或对白…",
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.7f),
                        )
                    },
                    shape = RoundedCornerShape(24.dp),
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedContainerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
                        unfocusedContainerColor = MaterialTheme.colorScheme.surfaceContainer,
                        disabledContainerColor = MaterialTheme.colorScheme.surfaceContainerLowest,
                        focusedBorderColor = MaterialTheme.colorScheme.primary,
                        unfocusedBorderColor = MaterialTheme.colorScheme.outlineVariant,
                    ),
                    maxLines = 5,
                    textStyle = MaterialTheme.typography.bodyLarge,
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Default),
                    keyboardActions = KeyboardActions(),
                    enabled = !state.busy,
                )

                // 交互状态按键组
                ComposerActionButton(
                    state = state,
                    onSend = onSend,
                    onCancel = onCancel,
                    onContinue = onContinue,
                    onRetry = onRetry,
                )
            }

            // 阻断或限制提示信息
            state.capabilities?.block?.message?.let { blockMsg ->
                Text(
                    text = blockMsg,
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.padding(start = dimens.space2),
                )
            }
        }
    }
}

/** 抉择与选项胶囊横向滚动栏 */
@Composable
private fun ActionChipsBar(
    state: StoryUiState,
    onChooseOption: (OptionDto) -> Unit,
    onToggleNote: () -> Unit,
    onOptionsMode: (String) -> Unit,
    onSwitchModel: (String) -> Unit,
    onSetReasoningEffort: (String) -> Unit,
    onOpenSettings: () -> Unit,
) {
    val scrollState = rememberScrollState()
    val latestOptions = state.messages.lastOrNull()?.options.orEmpty()

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .horizontalScroll(scrollState),
        horizontalArrangement = Arrangement.spacedBy(dimens.space2),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        // 模型快速选择胶囊
        ModelQuickSwitchChip(
            activeModelName = state.activePrimaryModelName,
            models = state.models,
            activeModelId = state.activePrimaryModel?.id,
            onSelectModel = onSwitchModel,
            onOpenSettings = onOpenSettings,
        )

        // 思考强度快速切换胶囊
        ThinkingEffortChip(
            effort = state.reasoningEffort,
            onSelectEffort = onSetReasoningEffort,
        )

        // 注记开关胶囊
        AssistChip(
            onClick = onToggleNote,
            label = {
                Text(
                    if (state.inputNote.isNotBlank()) "注记 (${state.inputNote.take(4)}…)" else "注记",
                    style = MaterialTheme.typography.labelMedium,
                )
            },
            leadingIcon = {
                Icon(
                    Icons.Filled.EditNote,
                    contentDescription = null,
                    modifier = Modifier.size(16.dp),
                    tint = if (state.inputNote.isNotBlank()) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                )
            },
            colors = AssistChipDefaults.assistChipColors(
                containerColor = if (state.noteVisible) MaterialTheme.colorScheme.primaryContainer else MaterialTheme.colorScheme.surfaceContainerHigh,
                labelColor = if (state.noteVisible) MaterialTheme.colorScheme.onPrimaryContainer else MaterialTheme.colorScheme.onSurface,
            ),
            shape = RoundedCornerShape(16.dp),
        )

        // 选项模式切换胶囊
        OptionsModeChip(mode = state.optionsMode, onSelect = onOptionsMode)

        // 若当前处于可行动状态，展示可立即选取的剧情选项胶囊
        if (!state.busy && latestOptions.isNotEmpty()) {
            latestOptions.forEach { option ->
                val intentColor = optionIntentColor(option.intent)
                AssistChip(
                    onClick = { onChooseOption(option) },
                    label = {
                        Text(
                            text = option.text,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            style = MaterialTheme.typography.labelMedium,
                        )
                    },
                    leadingIcon = {
                        Box(
                            modifier = Modifier
                                .size(8.dp)
                                .clip(CircleShape)
                                .background(intentColor),
                        )
                    },
                    colors = AssistChipDefaults.assistChipColors(
                        containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
                    ),
                    shape = RoundedCornerShape(16.dp),
                )
            }
        }
    }
}

/** 紧凑型导演/玩家注记输入卡片 */
@Composable
private fun NoteInputCard(
    note: String,
    onNoteChange: (String) -> Unit,
    onClose: () -> Unit,
) {
    Surface(
        color = MaterialTheme.colorScheme.surfaceContainerHigh,
        shape = RoundedCornerShape(16.dp),
        border = androidx.compose.foundation.BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(modifier = Modifier.padding(horizontal = dimens.space3, vertical = dimens.space2)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    text = "导演/玩家注记（对剧情推进的要求，非角色言行）",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.primary,
                    fontWeight = FontWeight.SemiBold,
                )
                IconButton(onClick = onClose, modifier = Modifier.size(24.dp)) {
                    Icon(Icons.Filled.Close, contentDescription = "收起注记", modifier = Modifier.size(16.dp))
                }
            }
            OutlinedTextField(
                value = note,
                onValueChange = onNoteChange,
                modifier = Modifier
                    .fillMaxWidth()
                    .heightIn(min = 40.dp, max = 90.dp),
                placeholder = {
                    Text(
                        "例如：描写更细腻一些 / 揭开身世伏笔 / 不要太快推进剧情",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.6f),
                    )
                },
                maxLines = 3,
                textStyle = MaterialTheme.typography.bodyMedium,
                colors = OutlinedTextFieldDefaults.colors(
                    focusedBorderColor = MaterialTheme.colorScheme.primary,
                    unfocusedBorderColor = Color.Transparent,
                ),
            )
        }
    }
}

/** 状态感知的主操作按钮（发送 / 中断 / 续写 / 重试） */
@Composable
private fun ComposerActionButton(
    state: StoryUiState,
    onSend: () -> Unit,
    onCancel: () -> Unit,
    onContinue: () -> Unit,
    onRetry: () -> Unit,
) {
    when {
        // 生成中或等待确认：显示中断按钮
        state.busy || state.phase == TurnPhase.AWAITING_CONFIRM -> {
            FilledIconButton(
                onClick = onCancel,
                colors = IconButtonDefaults.filledIconButtonColors(
                    containerColor = MaterialTheme.colorScheme.errorContainer,
                    contentColor = MaterialTheme.colorScheme.onErrorContainer,
                ),
                modifier = Modifier.size(48.dp),
            ) {
                Icon(Icons.Filled.Stop, contentDescription = "中断生成")
            }
        }
        // 长度截断：显示续写按钮
        state.phase == TurnPhase.TRUNCATED -> {
            Button(
                onClick = onContinue,
                shape = RoundedCornerShape(24.dp),
                colors = ButtonDefaults.buttonColors(
                    containerColor = MaterialTheme.colorScheme.secondary,
                    contentColor = MaterialTheme.colorScheme.onSecondary,
                ),
                modifier = Modifier.height(48.dp),
            ) {
                Icon(Icons.Filled.PlayArrow, contentDescription = null, modifier = Modifier.size(18.dp))
                Spacer(Modifier.width(4.dp))
                Text("续写")
            }
        }
        // 失败态：显示重试按钮
        state.phase == TurnPhase.FAILED -> {
            Button(
                onClick = onRetry,
                shape = RoundedCornerShape(24.dp),
                colors = ButtonDefaults.buttonColors(
                    containerColor = MaterialTheme.colorScheme.error,
                    contentColor = MaterialTheme.colorScheme.onError,
                ),
                modifier = Modifier.height(48.dp),
            ) {
                Icon(Icons.Filled.Refresh, contentDescription = null, modifier = Modifier.size(18.dp))
                Spacer(Modifier.width(4.dp))
                Text("重试")
            }
        }
        // 常规输入态：发送按钮
        else -> {
            val canSend = state.inputText.isNotBlank() && !state.busy
            FilledIconButton(
                onClick = onSend,
                enabled = canSend,
                colors = IconButtonDefaults.filledIconButtonColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    contentColor = MaterialTheme.colorScheme.onPrimary,
                    disabledContainerColor = MaterialTheme.colorScheme.surfaceContainerHighest,
                    disabledContentColor = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.4f),
                ),
                modifier = Modifier.size(48.dp),
            ) {
                Icon(Icons.AutoMirrored.Filled.Send, contentDescription = "发送")
            }
        }
    }
}

/** 选项模式切换胶囊 */
@Composable
private fun OptionsModeChip(mode: String, onSelect: (String) -> Unit) {
    var expanded by remember { mutableStateOf(false) }
    Box {
        AssistChip(
            onClick = { expanded = true },
            label = {
                Text(
                    when (mode) {
                        "always" -> "每轮给选项"
                        "never" -> "不给选项"
                        else -> "关键时刻的选项"
                    },
                    style = MaterialTheme.typography.labelMedium,
                )
            },
            leadingIcon = {
                Icon(Icons.Filled.Tune, contentDescription = null, modifier = Modifier.size(14.dp))
            },
            trailingIcon = {
                Icon(Icons.Filled.KeyboardArrowDown, contentDescription = null, modifier = Modifier.size(14.dp))
            },
            colors = AssistChipDefaults.assistChipColors(
                containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
            ),
            shape = RoundedCornerShape(16.dp),
        )
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            listOf(
                "auto" to "关键时刻给选项",
                "always" to "每轮给选项",
                "never" to "不给选项",
            ).forEach { (value, label) ->
                DropdownMenuItem(
                    text = { Text(label) },
                    onClick = {
                        expanded = false
                        onSelect(value)
                    },
                )
            }
        }
    }
}

/** 模型快速切换胶囊 */
@Composable
private fun ModelQuickSwitchChip(
    activeModelName: String,
    models: List<ModelInstanceDto>,
    activeModelId: String?,
    onSelectModel: (String) -> Unit,
    onOpenSettings: () -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    Box {
        AssistChip(
            onClick = { expanded = true },
            label = {
                Text(
                    text = "模型: $activeModelName",
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    style = MaterialTheme.typography.labelMedium,
                )
            },
            leadingIcon = {
                Icon(
                    Icons.Filled.SmartToy,
                    contentDescription = null,
                    modifier = Modifier.size(14.dp),
                    tint = if (activeModelId != null) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                )
            },
            trailingIcon = {
                Icon(Icons.Filled.KeyboardArrowDown, contentDescription = null, modifier = Modifier.size(14.dp))
            },
            colors = AssistChipDefaults.assistChipColors(
                containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
            ),
            shape = RoundedCornerShape(16.dp),
        )
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            if (models.isEmpty()) {
                DropdownMenuItem(
                    text = {
                        Column {
                            Text("还没有模型连接", style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Medium)
                            Text("前往设置配置模型连接", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    },
                    leadingIcon = {
                        Icon(Icons.Filled.Settings, contentDescription = null)
                    },
                    onClick = {
                        expanded = false
                        onOpenSettings()
                    },
                )
            } else {
                models.forEach { model ->
                    val isSelected = model.id == activeModelId
                    DropdownMenuItem(
                        text = {
                            Column {
                                Text(
                                    text = model.name.ifBlank { model.model ?: "未命名模型" },
                                    style = MaterialTheme.typography.bodyMedium,
                                    fontWeight = if (isSelected) FontWeight.Bold else FontWeight.Normal,
                                    color = if (isSelected) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurface,
                                )
                                val subtitle = listOfNotNull(
                                    model.model?.takeIf { it.isNotBlank() && it != model.name },
                                    when (model.kind) {
                                        "openai-chat" -> "OpenAI 兼容"
                                        "openai-responses" -> "OpenAI Responses"
                                        "anthropic" -> "Anthropic"
                                        else -> model.kind
                                    },
                                ).joinToString(" · ")
                                if (subtitle.isNotBlank()) {
                                    Text(
                                        text = subtitle,
                                        style = MaterialTheme.typography.labelSmall,
                                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    )
                                }
                            }
                        },
                        trailingIcon = if (isSelected) {
                            {
                                Icon(
                                    Icons.Filled.Check,
                                    contentDescription = "当前选择",
                                    tint = MaterialTheme.colorScheme.primary,
                                    modifier = Modifier.size(18.dp),
                                )
                            }
                        } else null,
                        onClick = {
                            expanded = false
                            onSelectModel(model.id)
                        },
                    )
                }
                HorizontalDivider(modifier = Modifier.padding(vertical = 4.dp))
                DropdownMenuItem(
                    text = { Text("管理模型连接…", style = MaterialTheme.typography.labelMedium) },
                    leadingIcon = {
                        Icon(Icons.Filled.Tune, contentDescription = null, modifier = Modifier.size(16.dp))
                    },
                    onClick = {
                        expanded = false
                        onOpenSettings()
                    },
                )
            }
        }
    }
}

/** 思考强度选择胶囊 */
@Composable
private fun ThinkingEffortChip(
    effort: String,
    onSelectEffort: (String) -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    val currentLabel = effortLabel(effort)
    val hasEffort = effort.isNotBlank()

    Box {
        AssistChip(
            onClick = { expanded = true },
            label = {
                Text(
                    text = "思考: $currentLabel",
                    style = MaterialTheme.typography.labelMedium,
                )
            },
            leadingIcon = {
                Icon(
                    Icons.Filled.Psychology,
                    contentDescription = null,
                    modifier = Modifier.size(14.dp),
                    tint = if (hasEffort) MaterialTheme.colorScheme.tertiary else MaterialTheme.colorScheme.onSurfaceVariant,
                )
            },
            trailingIcon = {
                Icon(Icons.Filled.KeyboardArrowDown, contentDescription = null, modifier = Modifier.size(14.dp))
            },
            colors = AssistChipDefaults.assistChipColors(
                containerColor = if (hasEffort) MaterialTheme.colorScheme.tertiaryContainer.copy(alpha = 0.6f) else MaterialTheme.colorScheme.surfaceContainerHigh,
                labelColor = if (hasEffort) MaterialTheme.colorScheme.onTertiaryContainer else MaterialTheme.colorScheme.onSurface,
            ),
            shape = RoundedCornerShape(16.dp),
        )
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            listOf(
                Triple("", "默认", "不发送思考参数，按模型默认行为"),
                Triple("low", "低", "思考最少，响应最快"),
                Triple("medium", "中", "速度与深度的折中"),
                Triple("high", "高", "思考最充分，深度推理"),
            ).forEach { (value, label, hint) ->
                val isSelected = value == effort
                DropdownMenuItem(
                    text = {
                        Column {
                            Text(
                                text = label,
                                style = MaterialTheme.typography.bodyMedium,
                                fontWeight = if (isSelected) FontWeight.Bold else FontWeight.Normal,
                                color = if (isSelected) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurface,
                            )
                            Text(
                                text = hint,
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    },
                    trailingIcon = if (isSelected) {
                        {
                            Icon(
                                Icons.Filled.Check,
                                contentDescription = "当前选择",
                                tint = MaterialTheme.colorScheme.primary,
                                modifier = Modifier.size(18.dp),
                            )
                        }
                    } else null,
                    onClick = {
                        expanded = false
                        onSelectEffort(value)
                    },
                )
            }
        }
    }
}

/** 历史快照只读提示条 */
@Composable
private fun ReadOnlyHistoryBar(onBackToHead: () -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(MaterialTheme.colorScheme.tertiaryContainer.copy(alpha = 0.6f))
            .padding(horizontal = dimens.space3, vertical = dimens.space2),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column {
            Text(
                "正在浏览历史记录",
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.onTertiaryContainer,
                fontWeight = FontWeight.SemiBold,
            )
            Text(
                "历史快照为只读状态，返回最新进度后即可继续对话",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onTertiaryContainer.copy(alpha = 0.8f),
            )
        }
        Button(
            onClick = onBackToHead,
            colors = ButtonDefaults.buttonColors(
                containerColor = MaterialTheme.colorScheme.tertiary,
                contentColor = MaterialTheme.colorScheme.onTertiary,
            ),
        ) {
            Text("返回最新")
        }
    }
}

@Composable
private fun optionIntentColor(intent: String): Color {
    val colors = LocalTavernColors.current
    return when (intent) {
        "aggressive" -> colors.intentAggressive
        "clever" -> colors.intentClever
        "emotional" -> colors.intentEmotional
        "chaotic" -> colors.intentChaotic
        else -> MaterialTheme.colorScheme.primary
    }
}
