package dev.tavernagent.mobile.ui.screens.story

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.animateContentSize
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.sizeIn
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.DisableSelection
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.VolumeUp
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Casino
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
import androidx.compose.material.icons.filled.FormatQuote
import androidx.compose.material.icons.filled.Psychology
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.tavernagent.mobile.data.remote.dto.OptionDto
import dev.tavernagent.mobile.data.remote.dto.TextBlockDto
import dev.tavernagent.mobile.domain.CardMarkerParser
import dev.tavernagent.mobile.domain.ContentSegment
import dev.tavernagent.mobile.domain.DraftBlock
import dev.tavernagent.mobile.domain.StoryMessage
import dev.tavernagent.mobile.ui.components.CardImage
import dev.tavernagent.mobile.ui.components.StoryAvatar
import dev.tavernagent.mobile.ui.components.relativeTime
import dev.tavernagent.mobile.ui.theme.LocalTavernColors
import dev.tavernagent.mobile.ui.theme.TavernColors
import dev.tavernagent.mobile.ui.theme.dimens
import dev.tavernagent.mobile.ui.theme.texts

/**
 * 沉浸式酒馆会话消息项：
 * - 玩家行动气泡（靠右对齐 + 玩家头像 + 注记与检定）
 * - 角色演绎气泡（靠左对齐 + 角色头像 + 思考引文折叠 + 多模态立绘与排版 + 快捷操作）
 */
@Composable
fun StoryMessageItem(
    message: StoryMessage,
    colors: TavernColors,
    resolveImage: (String) -> String?,
    showOptions: Boolean,
    busy: Boolean,
    onChooseOption: (OptionDto) -> Unit,
    onReadAloud: (String) -> Unit,
    onRegenerate: () -> Unit,
    onFork: () -> Unit,
    onViewAt: () -> Unit,
    characterName: String = "角色",
    characterAvatarUrl: String? = null,
    playerName: String = "玩家",
    playerAvatarUrl: String? = null,
    onImageClick: (String) -> Unit = {},
    onQuote: ((String) -> Unit)? = null,
    onOpenCharacterDetails: (() -> Unit)? = null,
) {
    Column(
        modifier = Modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(dimens.space3),
    ) {
        // 1. 玩家发言 / 行动气泡（非开场节点且有文本输入时展示）
        if (!message.isRoot && message.inputText.isNotBlank()) {
            PlayerTurnBubble(
                text = message.inputText,
                note = message.inputNote,
                playerName = playerName,
                avatarUrl = playerAvatarUrl,
                turnNumber = message.turnNumber,
                createdAt = message.createdAt,
                hasChecks = message.changes.isNotEmpty(),
                onQuote = onQuote,
            )
        }

        // 2. 思考引文块（若本轮有独立思考内容）
        if (!message.thinking.isNullOrBlank()) {
            ThinkingBlock(
                thinking = message.thinking,
                seconds = message.thinkingSeconds,
                streaming = false,
            )
        }

        // 3. AI / 角色回复气泡
        if (message.blocks.isNotEmpty() || message.isRoot) {
            AssistantTurnBubble(
                message = message,
                characterName = characterName,
                characterAvatarUrl = characterAvatarUrl,
                colors = colors,
                resolveImage = resolveImage,
                onReadAloud = { onReadAloud(message.blocks.joinToString("\n") { it.text }) },
                onImageClick = onImageClick,
                onQuote = onQuote,
                onRegenerate = onRegenerate,
                onFork = onFork,
                onViewAt = onViewAt,
                onOpenCharacterDetails = onOpenCharacterDetails,
            )
        }

        // 4. 场景与状态 UI Hints
        if (message.uiHints.isNotEmpty()) {
            HintRow(hints = message.uiHints)
        }

        // 5. 最新一轮待选取的抉择卡片
        if (showOptions && message.options.isNotEmpty()) {
            ChoiceList(options = message.options, enabled = !busy, onChoose = onChooseOption)
        }
    }
}

/**
 * 玩家消息的乐观回显：发送成功即出现，提交后由服务端节点接管。
 * 复用与正式消息一致的玩家气泡，避免「发完要等整轮生成才看到自己的话」。
 */
@Composable
fun PlayerEchoBubble(
    text: String,
    note: String?,
    playerName: String,
    avatarUrl: String?,
) {
    PlayerTurnBubble(
        text = text,
        note = note,
        playerName = playerName,
        avatarUrl = avatarUrl,
        turnNumber = 0,
        createdAt = null,
        hasChecks = false,
        onQuote = null,
    )
}

/** 玩家行动气泡（靠右排列，凸显玩家第一人称行动） */
@Composable
private fun PlayerTurnBubble(
    text: String,
    note: String?,
    playerName: String,
    avatarUrl: String?,
    turnNumber: Int,
    createdAt: String?,
    hasChecks: Boolean,
    onQuote: ((String) -> Unit)?,
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.End,
        verticalAlignment = Alignment.Top,
    ) {
        Column(
            horizontalAlignment = Alignment.End,
            modifier = Modifier.weight(1f, fill = false).padding(end = dimens.space2),
        ) {
            // 玩家信息头
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(dimens.space1),
                modifier = Modifier.padding(bottom = 2.dp),
            ) {
                if (hasChecks) {
                    Icon(
                        Icons.Filled.Casino,
                        contentDescription = "含行动检定",
                        tint = MaterialTheme.colorScheme.tertiary,
                        modifier = Modifier.size(13.dp),
                    )
                }
                Text(
                    text = playerName,
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.primary,
                    fontWeight = FontWeight.SemiBold,
                )
                Text(
                    text = messageStamp(createdAt, turnNumber),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.7f),
                )
            }

            // 气泡卡片主体
            Surface(
                color = MaterialTheme.colorScheme.primaryContainer,
                shape = RoundedCornerShape(topStart = 16.dp, topEnd = 4.dp, bottomStart = 16.dp, bottomEnd = 16.dp),
                border = androidx.compose.foundation.BorderStroke(1.dp, MaterialTheme.colorScheme.primary.copy(alpha = 0.3f)),
                modifier = Modifier.widthIn(max = 340.dp),
            ) {
                SelectionContainer {
                    Column(modifier = Modifier.padding(horizontal = dimens.space3, vertical = dimens.space2)) {
                        // 若有玩家注记，在气泡顶端呈现微型注记条
                        if (!note.isNullOrBlank()) {
                            DisableSelection {
                                Surface(
                                    color = MaterialTheme.colorScheme.surface.copy(alpha = 0.25f),
                                    shape = RoundedCornerShape(8.dp),
                                    modifier = Modifier.fillMaxWidth().padding(bottom = dimens.space1),
                                ) {
                                    Text(
                                        text = "注记：$note",
                                        style = MaterialTheme.typography.labelSmall,
                                        color = MaterialTheme.colorScheme.onPrimaryContainer.copy(alpha = 0.85f),
                                        modifier = Modifier.padding(horizontal = 8.dp, vertical = 3.dp),
                                    )
                                }
                            }
                        }

                        // 玩家言行正文
                        Text(
                            text = inlineMarkdown(text),
                            style = texts.playerSpeech.copy(color = MaterialTheme.colorScheme.onPrimaryContainer),
                        )
                    }
                }
            }
        }

        // 玩家头像
        StoryAvatar(
            name = playerName,
            avatarUrl = avatarUrl,
            size = 38.dp,
            borderColor = MaterialTheme.colorScheme.primary.copy(alpha = 0.5f),
        )
    }
}

/** 助手/角色演绎气泡（靠左排列，结构清晰，多模态混排） */
@Composable
private fun AssistantTurnBubble(
    message: StoryMessage,
    characterName: String,
    characterAvatarUrl: String?,
    colors: TavernColors,
    resolveImage: (String) -> String?,
    onReadAloud: () -> Unit,
    onImageClick: (String) -> Unit,
    onQuote: ((String) -> Unit)?,
    onRegenerate: () -> Unit,
    onFork: () -> Unit,
    onViewAt: () -> Unit,
    onOpenCharacterDetails: (() -> Unit)?,
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.Start,
        verticalAlignment = Alignment.Top,
    ) {
        // 角色头像（点击可唤起角色属性详情）
        StoryAvatar(
            name = characterName,
            avatarUrl = characterAvatarUrl,
            size = 40.dp,
            onClick = onOpenCharacterDetails,
            borderColor = MaterialTheme.colorScheme.outlineVariant,
        )

        Spacer(Modifier.width(dimens.space2))

        Column(modifier = Modifier.weight(1f)) {
            // 角色信息栏与微工具栏
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(dimens.space1),
                ) {
                    Text(
                        text = if (message.isRoot) "命运序章" else characterName,
                        style = MaterialTheme.typography.titleSmall,
                        fontWeight = FontWeight.Bold,
                        color = MaterialTheme.colorScheme.onSurface,
                    )
                    if (message.isRoot) {
                        Surface(
                            color = MaterialTheme.colorScheme.tertiaryContainer,
                            shape = RoundedCornerShape(4.dp),
                        ) {
                            Text(
                                text = "场景起步",
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onTertiaryContainer,
                                modifier = Modifier.padding(horizontal = 4.dp, vertical = 1.dp),
                            )
                        }
                    } else {
                        Text(
                            text = messageStamp(message.createdAt, message.turnNumber),
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.6f),
                        )
                    }
                }

                // 快捷工具栏（朗读、引用、从此分叉）
                Row(verticalAlignment = Alignment.CenterVertically) {
                    IconButton(onClick = onReadAloud) {
                        Icon(
                            Icons.AutoMirrored.Filled.VolumeUp,
                            contentDescription = "朗读",
                            tint = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.7f),
                            modifier = Modifier.size(16.dp),
                        )
                    }
                    if (onQuote != null && message.blocks.isNotEmpty()) {
                        IconButton(
                            onClick = { onQuote(message.blocks.joinToString("\n") { it.text }) },
                        ) {
                            Icon(
                                Icons.Filled.FormatQuote,
                                contentDescription = "引用",
                                tint = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.7f),
                                modifier = Modifier.size(16.dp),
                            )
                        }
                    }
                }
            }

            Spacer(Modifier.height(4.dp))

            // 消息气泡卡片主体
            Surface(
                color = MaterialTheme.colorScheme.surfaceContainer,
                shape = RoundedCornerShape(topStart = 4.dp, topEnd = 16.dp, bottomStart = 16.dp, bottomEnd = 16.dp),
                border = androidx.compose.foundation.BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.5f)),
                modifier = Modifier.fillMaxWidth(),
            ) {
                SelectionContainer {
                    Column(
                        modifier = Modifier.padding(horizontal = dimens.space3, vertical = dimens.space3),
                        verticalArrangement = Arrangement.spacedBy(dimens.space2),
                    ) {
                        message.blocks.forEach { block ->
                            BlockParagraph(
                                block = block,
                                colors = colors,
                                resolveImage = resolveImage,
                                onImageClick = onImageClick,
                            )
                        }
                    }
                }
            }

            // 底部操作链接（重演、从此分叉、查看节点）
            if (!message.isRoot) {
                TurnActionsRow(
                    onRegenerate = onRegenerate,
                    onFork = onFork,
                    onViewAt = onViewAt,
                )
            }
        }
    }
}

/** 流式草稿实时展示卡 */
@Composable
fun StreamingDraft(
    blocks: List<DraftBlock>,
    colors: TavernColors,
    resolveImage: (String) -> String?,
    characterName: String = "正在构想…",
    characterAvatarUrl: String? = null,
    onImageClick: (String) -> Unit = {},
) {
    if (blocks.isEmpty()) return

    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.Start,
        verticalAlignment = Alignment.Top,
    ) {
        StoryAvatar(
            name = characterName,
            avatarUrl = characterAvatarUrl,
            size = 40.dp,
            borderColor = MaterialTheme.colorScheme.primary,
        )

        Spacer(Modifier.width(dimens.space2))

        Column(modifier = Modifier.weight(1f)) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(dimens.space1),
                modifier = Modifier.padding(bottom = 4.dp),
            ) {
                Text(
                    text = characterName,
                    style = MaterialTheme.typography.titleSmall,
                    fontWeight = FontWeight.Bold,
                )
                CircularProgressIndicator(modifier = Modifier.size(12.dp), strokeWidth = 2.dp)
                Text(
                    text = "正在演绎…",
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.primary,
                )
            }

            Surface(
                color = MaterialTheme.colorScheme.surfaceContainer,
                shape = RoundedCornerShape(topStart = 4.dp, topEnd = 16.dp, bottomStart = 16.dp, bottomEnd = 16.dp),
                border = androidx.compose.foundation.BorderStroke(1.dp, MaterialTheme.colorScheme.primary.copy(alpha = 0.5f)),
                modifier = Modifier.fillMaxWidth(),
            ) {
                SelectionContainer {
                    Column(
                        modifier = Modifier.padding(horizontal = dimens.space3, vertical = dimens.space3),
                        verticalArrangement = Arrangement.spacedBy(dimens.space2),
                    ) {
                        blocks.forEach { block ->
                            BlockParagraph(
                                block = TextBlockDto(kind = block.kind, speakerId = block.speakerId, text = block.text),
                                colors = colors,
                                resolveImage = resolveImage,
                                onImageClick = onImageClick,
                            )
                        }
                    }
                }
            }
        }
    }
}

/** 智能段落分段与多模态立绘渲染 */
@Composable
private fun BlockParagraph(
    block: TextBlockDto,
    colors: TavernColors,
    resolveImage: (String) -> String?,
    onImageClick: (String) -> Unit,
) {
    // 经由 CardMarkerParser 统一解析正文中的图片标记、Markdown 图片、HUD 与场景。
    // resolveImage 必须一起作为 remember 的键：资源表是异步到达的，只认 block.text 会把
    // 「表尚未就绪时解析出的空结果」永久钉死，图片只能靠列表项回收才重来一次。
    val segments = remember(block.text, resolveImage) {
        CardMarkerParser.parse(block.text, resolveImage)
    }

    if (segments.isEmpty()) {
        RenderPureTextBlock(kind = block.kind, text = block.text, colors = colors)
    } else {
        segments.forEach { seg ->
            when (seg) {
                is ContentSegment.Text -> RenderPureTextBlock(kind = block.kind, text = seg.content, colors = colors)
                is ContentSegment.Image -> DisableSelection {
                    CardImage(
                        url = seg.url,
                        rawKey = seg.rawKey.ifBlank { seg.alt },
                        onOpen = onImageClick,
                    )
                }
                is ContentSegment.Scene -> DisableSelection {
                    Surface(
                        color = MaterialTheme.colorScheme.surfaceVariant,
                        shape = RoundedCornerShape(8.dp),
                        modifier = Modifier.fillMaxWidth().padding(vertical = 2.dp),
                    ) {
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(4.dp),
                            modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp),
                        ) {
                            Icon(Icons.Filled.AutoAwesome, contentDescription = null, modifier = Modifier.size(13.dp), tint = MaterialTheme.colorScheme.primary)
                            Text(text = "场景：${seg.title}", style = MaterialTheme.typography.labelSmall)
                        }
                    }
                }
                is ContentSegment.Hud -> DisableSelection { CardHUDMini(values = seg.values) }
                is ContentSegment.Thought -> DisableSelection { ThinkingBlock(thinking = seg.thought, seconds = null, streaming = false) }
            }
        }
    }
}

@Composable
private fun RenderPureTextBlock(kind: String, text: String, colors: TavernColors) {
    if (text.isBlank()) return
    when (kind) {
        "dialogue" -> Text(
            text = "「${text.trim().trim('「', '」')}」",
            style = texts.dialogue.copy(color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Medium, lineHeight = 22.sp),
        )
        "inner_monologue" -> Text(
            text = "（${text.trim().trim('（', '）')}）",
            style = texts.innerMonologue.copy(color = MaterialTheme.colorScheme.onSurfaceVariant, fontStyle = FontStyle.Italic, lineHeight = 20.sp),
        )
        else -> Row(modifier = Modifier.fillMaxWidth()) {
            Box(modifier = Modifier.width(3.dp).heightIn(min = 18.dp).background(MaterialTheme.colorScheme.primary.copy(alpha = 0.5f), RoundedCornerShape(2.dp)))
            Spacer(Modifier.width(dimens.space2))
            Text(text = inlineMarkdown(text), style = texts.narration.copy(color = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.9f), lineHeight = 22.sp))
        }
    }
}

/** 思考引文块：支持折叠/展开动画、时长提示、等宽字体与呼吸效果 */
@Composable
fun ThinkingBlock(thinking: String, seconds: Int?, streaming: Boolean) {
    var expanded by remember { mutableStateOf(false) }
    val tail = thinking.trim().lineSequence().lastOrNull().orEmpty()

    val alphaPulse = if (streaming) {
        val infiniteTransition = rememberInfiniteTransition(label = "pulse")
        val value by infiniteTransition.animateFloat(
            initialValue = 0.6f,
            targetValue = 1.0f,
            animationSpec = infiniteRepeatable(tween(800), repeatMode = RepeatMode.Reverse),
            label = "pulseAlpha",
        )
        value
    } else {
        // 非流式（历史/已完成）时不要创建无限动画：否则每条消息都持续调度帧与重组。
        1f
    }

    Surface(
        modifier = Modifier.fillMaxWidth(),
        color = MaterialTheme.colorScheme.surfaceContainerLowest,
        shape = RoundedCornerShape(12.dp),
        border = androidx.compose.foundation.BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.4f)),
    ) {
        Column(modifier = Modifier.animateContentSize()) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .clickable { expanded = !expanded }
                    .padding(horizontal = dimens.space3, vertical = dimens.space2),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(dimens.space2),
            ) {
                Icon(
                    Icons.Filled.Psychology,
                    contentDescription = null,
                    tint = if (streaming) MaterialTheme.colorScheme.tertiary else MaterialTheme.colorScheme.primary,
                    modifier = Modifier.size(16.dp).alpha(if (streaming) alphaPulse else 1.0f),
                )
                Text(
                    text = buildString {
                        append("思考过程 (Reasoning)")
                        if (streaming) append(" · 构想演绎中…")
                        else if (seconds != null) append(" · 用时 ${seconds} 秒")
                    },
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.weight(1f),
                )
                Icon(
                    if (expanded) Icons.Filled.ExpandLess else Icons.Filled.ExpandMore,
                    contentDescription = if (expanded) "收起" else "展开",
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.size(18.dp),
                )
            }

            if (streaming && !expanded && tail.isNotBlank()) {
                Text(
                    text = tail,
                    style = texts.thinking.copy(fontFamily = FontFamily.Monospace, fontSize = 12.sp),
                    color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.8f),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.padding(horizontal = dimens.space3, vertical = 2.dp),
                )
            }

            AnimatedVisibility(visible = expanded) {
                SelectionContainer {
                    Text(
                        text = thinking.ifBlank { "（无思考过程输出）" },
                        style = texts.thinking.copy(
                            fontFamily = FontFamily.Monospace,
                            fontSize = 12.sp,
                            lineHeight = 18.sp,
                        ),
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier
                            .heightIn(max = 280.dp)
                            .verticalScroll(rememberScrollState())
                            .padding(horizontal = dimens.space3, vertical = dimens.space2),
                    )
                }
            }
        }
    }
}

/** 迷你 HUD 卡片 */
@Composable
private fun CardHUDMini(values: Map<String, String>) {
    Surface(
        color = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.7f),
        shape = RoundedCornerShape(8.dp),
        modifier = Modifier.fillMaxWidth().padding(vertical = 2.dp),
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            values.entries.take(4).forEach { (k, v) ->
                Text(
                    text = "$k: $v",
                    style = MaterialTheme.typography.labelSmall,
                    fontWeight = FontWeight.Medium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

/** 消息时间戳：优先相对时间，解析不到时退化为「第 N 轮」。 */
private fun messageStamp(createdAt: String?, turnNumber: Int): String {
    val time = relativeTime(createdAt)
    if (time.isNotBlank()) return time
    return if (turnNumber > 0) "第 $turnNumber 轮" else ""
}

/** 极简内联 Markdown 解析（粗体、斜体、行内代码） */
private val inlineBoldRe = Regex("\\*\\*(.+?)\\*\\*")
private val inlineItalicRe = Regex("(?<!\\*)\\*(?!\\*)(.+?)(?<!\\*)\\*(?!\\*)")
private val inlineCodeRe = Regex("`([^`]+)`")

fun inlineMarkdown(text: String): AnnotatedString = buildAnnotatedString {
    var index = 0

    val matches = (inlineBoldRe.findAll(text) + inlineItalicRe.findAll(text) + inlineCodeRe.findAll(text))
        .sortedBy { it.range.first }

    for (match in matches) {
        if (match.range.first < index) continue
        append(text.substring(index, match.range.first))

        val matchedText = match.value
        when {
            matchedText.startsWith("**") -> {
                pushStyle(SpanStyle(fontWeight = FontWeight.Bold))
                append(match.groupValues[1])
                pop()
            }
            matchedText.startsWith("`") -> {
                pushStyle(SpanStyle(fontFamily = FontFamily.Monospace, background = Color(0x22888888)))
                append(match.groupValues[1])
                pop()
            }
            else -> {
                pushStyle(SpanStyle(fontStyle = FontStyle.Italic))
                append(match.groupValues[1])
                pop()
            }
        }
        index = match.range.last + 1
    }
    if (index < text.length) append(text.substring(index))
}

/** 剧情抉择卡片 */
@Composable
fun ChoiceList(options: List<OptionDto>, enabled: Boolean, onChoose: (OptionDto) -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(dimens.space2)) {
        Text("可执行抉择", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        options.forEach { option ->
            val intentColor = intentColor(option.intent)
            Card(
                modifier = Modifier
                    .fillMaxWidth()
                    .clickable(enabled = enabled) { onChoose(option) },
                shape = RoundedCornerShape(12.dp),
                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh),
                border = androidx.compose.foundation.BorderStroke(1.dp, intentColor.copy(alpha = 0.35f)),
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Box(
                        modifier = Modifier
                            .width(4.dp)
                            .height(50.dp)
                            .background(intentColor),
                    )
                    Column(
                        modifier = Modifier.padding(horizontal = dimens.space3, vertical = dimens.space2),
                        verticalArrangement = Arrangement.spacedBy(2.dp),
                    ) {
                        Text(
                            text = intentLabel(option.intent),
                            style = MaterialTheme.typography.labelSmall,
                            color = intentColor,
                            fontWeight = FontWeight.Bold,
                        )
                        Text(text = option.text, style = MaterialTheme.typography.bodyMedium)
                    }
                }
            }
        }
    }
}

@Composable
private fun intentColor(intent: String): Color {
    val colors = LocalTavernColors.current
    return when (intent) {
        "aggressive" -> colors.intentAggressive
        "clever" -> colors.intentClever
        "emotional" -> colors.intentEmotional
        "chaotic" -> colors.intentChaotic
        else -> MaterialTheme.colorScheme.primary
    }
}

private fun intentLabel(intent: String): String = when (intent) {
    "aggressive" -> "强势威慑"
    "clever" -> "机智试探"
    "emotional" -> "共情抚慰"
    "chaotic" -> "出其不意"
    else -> "行动选项"
}

@Composable
private fun HintRow(hints: List<dev.tavernagent.mobile.data.remote.dto.UiHintDto>) {
    Row(
        horizontalArrangement = Arrangement.spacedBy(dimens.space2),
        modifier = Modifier.fillMaxWidth(),
    ) {
        hints.take(3).forEach { hint ->
            val label = when (hint.kind) {
                "scene" -> hint.values["title"] ?: hint.text ?: "场景"
                "hud" -> hint.values.entries.take(2).joinToString(" ") { "${it.key}:${it.value}" }
                else -> hint.text.orEmpty()
            }
            if (label.isNotBlank()) {
                Surface(
                    color = MaterialTheme.colorScheme.surfaceContainerHigh,
                    shape = RoundedCornerShape(999.dp),
                ) {
                    Text(
                        text = label,
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(horizontal = dimens.space2, vertical = 3.dp),
                    )
                }
            }
        }
    }
}

@Composable
private fun TurnActionsRow(onRegenerate: () -> Unit, onFork: () -> Unit, onViewAt: () -> Unit) {
    Row(
        horizontalArrangement = Arrangement.spacedBy(dimens.space1),
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier.padding(top = 2.dp),
    ) {
        TextButtonMini("重演生成", onRegenerate)
        TextButtonMini("从此分叉", onFork)
        TextButtonMini("回溯查看", onViewAt)
    }
}

@Composable
private fun TextButtonMini(label: String, onClick: () -> Unit) {
    Box(
        modifier = Modifier
            .sizeIn(minWidth = 48.dp, minHeight = 48.dp)
            .clip(RoundedCornerShape(4.dp))
            .clickable(role = Role.Button, onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            text = label,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.75f),
        )
    }
}
