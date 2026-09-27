package dev.tavernagent.mobile.ui.screens.branch

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
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
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.nativeCanvas
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.tavernagent.mobile.data.remote.dto.GraphNodeDto
import dev.tavernagent.mobile.data.remote.dto.PlotNodeDto
import dev.tavernagent.mobile.ui.components.EmptyState
import dev.tavernagent.mobile.ui.components.ErrorCard
import dev.tavernagent.mobile.ui.components.LoadingBox
import dev.tavernagent.mobile.ui.theme.dimens

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BranchTreeScreen(
    sessionId: String,
    branchId: String,
    onOpenNode: (sessionId: String, branchId: String, nodeId: String) -> Unit,
    onBranchChanged: (branchId: String) -> Unit,
    onBack: () -> Unit,
) {
    val viewModel: BranchTreeViewModel = viewModel(factory = BranchTreeViewModel.factory())
    val state by viewModel.state.collectAsStateWithLifecycle()
    var mode by remember { mutableStateOf("分支") }
    var forkTarget by remember { mutableStateOf<PlotNodeDto?>(null) }
    var graphSelection by remember { mutableStateOf<GraphNodeDto?>(null) }

    LaunchedEffect(sessionId, branchId) { viewModel.load(sessionId, branchId) }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回")
                    }
                },
                title = { Text("分支与回溯") },
                actions = {
                    Row(horizontalArrangement = Arrangement.spacedBy(dimens.space1)) {
                        listOf("分支", "图谱").forEach { label ->
                            FilterChip(
                                selected = mode == label,
                                onClick = { mode = label },
                                label = { Text(label) },
                            )
                        }
                    }
                },
            )
        },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            val view = state.view
            when {
                state.loading -> LoadingBox()
                view == null -> ErrorCard(
                    message = state.error ?: "无法加载分支",
                    modifier = Modifier.padding(dimens.space4),
                    onRetry = { viewModel.load(sessionId, branchId) },
                )
                mode == "分支" -> LazyColumn(
                    modifier = Modifier.fillMaxSize(),
                    contentPadding = androidx.compose.foundation.layout.PaddingValues(dimens.space4),
                    verticalArrangement = Arrangement.spacedBy(dimens.space2),
                ) {
                    state.error?.let { message ->
                        item { ErrorCard(message = message, onRetry = { viewModel.load(sessionId, branchId) }) }
                    }
                    items(view.nodes, key = { it.nodeId }) { node ->
                        NodeRow(
                            node = node,
                            label = viewModel.describe(node),
                            description = nodeDescription(node),
                            isActive = node.nodeId == view.branch.headNodeId,
                            isViewing = node.nodeId == view.viewNodeId,
                            onOpen = { onOpenNode(sessionId, view.branch.branchId, node.nodeId) },
                            onFork = { forkTarget = node },
                        )
                    }
                }
                else -> GraphCanvas(
                    nodes = state.graph?.nodes.orEmpty(),
                    targetNodeId = state.graph?.targetNodeId.orEmpty(),
                    loading = state.graphLoading,
                    onSelect = { graphSelection = it },
                )
            }
        }
    }

    forkTarget?.let { node ->
        AlertDialog(
            onDismissRequest = { forkTarget = null },
            title = { Text("从此处开辟分支") },
            text = { Text("将从「${viewModel.describe(node)}」建立新分支，不生成新内容。原分支不受影响。") },
            confirmButton = {
                Button(
                    onClick = {
                        forkTarget = null
                        viewModel.fork(node.nodeId, onBranchChanged)
                    },
                    enabled = !state.busy,
                ) { Text("开辟分支") }
            },
            dismissButton = { TextButton(onClick = { forkTarget = null }) { Text("取消") } },
        )
    }

    graphSelection?.let { node ->
        AlertDialog(
            onDismissRequest = { graphSelection = null },
            title = { Text("消息详情") },
            text = { Text("${node.leafLabel()} · 深度 ${node.depth} · 子节点 ${node.childCount}") },
            confirmButton = {
                Button(onClick = {
                    graphSelection = null
                    onOpenNode(sessionId, state.view?.branch?.branchId ?: branchId, node.nodeId)
                }) { Text("查看") }
            },
            dismissButton = { TextButton(onClick = { graphSelection = null }) { Text("关闭") } },
        )
    }
}

@Composable
private fun NodeRow(
    node: PlotNodeDto,
    label: String,
    description: String,
    isActive: Boolean,
    isViewing: Boolean,
    onOpen: () -> Unit,
    onFork: () -> Unit,
) {
    Card(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onOpen),
        colors = CardDefaults.cardColors(
            containerColor = if (isViewing) MaterialTheme.colorScheme.primaryContainer else MaterialTheme.colorScheme.surfaceContainerHigh,
        ),
    ) {
        Row(modifier = Modifier.padding(dimens.space3), verticalAlignment = Alignment.CenterVertically) {
            Box(
                modifier = Modifier
                    .width(3.dp)
                    .height(40.dp)
                    .background(
                        when {
                            isViewing -> MaterialTheme.colorScheme.primary
                            isActive -> MaterialTheme.colorScheme.secondary
                            else -> MaterialTheme.colorScheme.outlineVariant
                        },
                    ),
            )
            Spacer(Modifier.width(dimens.space3))
            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Row(horizontalArrangement = Arrangement.spacedBy(dimens.space2), verticalAlignment = Alignment.CenterVertically) {
                    Text(label, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.SemiBold)
                    if (isActive) StatusPill("活跃中", MaterialTheme.colorScheme.secondary)
                    if (isViewing) StatusPill("查看中", MaterialTheme.colorScheme.primary)
                }
                Text(
                    text = description,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            TextButton(onClick = onFork) { Text("分叉") }
        }
    }
}

@Composable
private fun StatusPill(text: String, color: Color) {
    Box(
        modifier = Modifier
            .background(color.copy(alpha = 0.15f), RoundedCornerShape(999.dp))
            .padding(horizontal = dimens.space2, vertical = 2.dp),
    ) {
        Text(text, style = MaterialTheme.typography.labelSmall, color = color)
    }
}

private fun nodeDescription(node: PlotNodeDto): String {
    if (node.kind == "root") {
        val root = dev.tavernagent.mobile.data.remote.ApiJson.decodeOrNull<Map<String, String>>(node.contentJson)
        return (root?.get("openingText") ?: "开场白").take(60)
    }
    val content = node.turnContent()
    val input = content?.inputText.orEmpty()
    if (input.isNotBlank()) return "玩家：${input.take(48)}"
    val first = content?.blocks?.firstOrNull()?.text.orEmpty()
    return first.ifBlank { "文学演义片段" }.take(56)
}

private fun GraphNodeDto.leafLabel(): String = when (kind) {
    "root" -> "对话起点"
    "memory_change" -> "记忆整理"
    "director_event" -> "导演安排"
    else -> "回合 #$turnNumber"
}

/** 简易分支图谱：按深度分列，贝塞尔连线；点击节点查看详情。 */
@Composable
private fun GraphCanvas(
    nodes: List<GraphNodeDto>,
    targetNodeId: String,
    loading: Boolean,
    onSelect: (GraphNodeDto) -> Unit,
) {
    if (loading && nodes.isEmpty()) {
        LoadingBox()
        return
    }
    if (nodes.isEmpty()) {
        EmptyState(title = "没有可展示的分支", hint = "图谱会以目标节点为中心取局部子图。")
        return
    }
    val density = LocalDensity.current
    val nodeWidth = with(density) { 140.dp.toPx() }
    val nodeHeight = with(density) { 56.dp.toPx() }
    val columnGap = with(density) { 76.dp.toPx() }
    val rowGap = with(density) { 28.dp.toPx() }
    val padding = with(density) { 24.dp.toPx() }

    val byDepth = nodes.groupBy { it.depth }.toSortedMap()
    val positions = remember(nodes) {
        val map = mutableMapOf<String, Offset>()
        byDepth.forEach { (depth, column) ->
            column.sortedBy { it.turnNumber }.forEachIndexed { index, node ->
                map[node.nodeId] = Offset(
                    x = padding + depth * (nodeWidth + columnGap),
                    y = padding + index * (nodeHeight + rowGap),
                )
            }
        }
        map
    }
    val maxDepth = byDepth.keys.maxOrNull() ?: 0
    val maxRows = byDepth.values.maxOfOrNull { it.size } ?: 1
    val contentWidth = padding * 2 + (maxDepth + 1) * nodeWidth + maxDepth * columnGap
    val contentHeight = padding * 2 + maxRows * nodeHeight + (maxRows - 1) * rowGap

    val colors = MaterialTheme.colorScheme
    val labelColor = colors.onSurface
    val textPaint = remember {
        android.graphics.Paint().apply {
            isAntiAlias = true
            textSize = 32f
        }
    }
    var canvasSize by remember { mutableStateOf(Size.Zero) }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .horizontalScroll(rememberScrollState()),
    ) {
        Canvas(
            modifier = Modifier
                .width(with(density) { contentWidth.toDp() })
                .height(with(density) { contentHeight.toDp() })
                .pointerInput(positions) {
                    detectTapGestures { offset ->
                        val hit = positions.minByOrNull { (_, position) ->
                            (position - offset).getDistance()
                        }
                        val chosen = hit?.takeIf { (_, position) ->
                            offset.x in position.x..(position.x + nodeWidth) &&
                                offset.y in position.y..(position.y + nodeHeight)
                        }
                        chosen?.let { (id, _) -> nodes.firstOrNull { it.nodeId == id }?.let(onSelect) }
                    }
                },
        ) {
            canvasSize = size
            // 连线
            nodes.forEach { node ->
                val from = positions[node.nodeId] ?: return@forEach
                val parent = node.parentId?.let { positions[it] } ?: return@forEach
                val path = Path().apply {
                    val startX = parent.x + nodeWidth
                    val startY = parent.y + nodeHeight / 2
                    val endX = from.x
                    val endY = from.y + nodeHeight / 2
                    moveTo(startX, startY)
                    cubicTo(startX + columnGap / 2, startY, endX - columnGap / 2, endY, endX, endY)
                }
                drawPath(path, color = colors.outlineVariant, style = androidx.compose.ui.graphics.drawscope.Stroke(width = 2f))
            }
            // 节点
            nodes.forEach { node ->
                val position = positions[node.nodeId] ?: return@forEach
                val isTarget = node.nodeId == targetNodeId
                val fill = when {
                    isTarget -> colors.primaryContainer
                    node.isCandidate -> colors.tertiaryContainer
                    else -> colors.surfaceContainerHigh
                }
                drawRoundRect(
                    color = fill,
                    topLeft = position,
                    size = Size(nodeWidth, nodeHeight),
                    cornerRadius = androidx.compose.ui.geometry.CornerRadius(12f, 12f),
                )
                drawRoundRect(
                    color = if (isTarget) colors.primary else colors.outlineVariant,
                    topLeft = position,
                    size = Size(nodeWidth, nodeHeight),
                    cornerRadius = androidx.compose.ui.geometry.CornerRadius(12f, 12f),
                    style = androidx.compose.ui.graphics.drawscope.Stroke(width = if (isTarget) 4f else 2f),
                )
                textPaint.color = labelColor.copy(alpha = 1f).toArgb()
                drawContext.canvas.nativeCanvas.drawText(
                    node.leafLabel(),
                    position.x + 16f,
                    position.y + 36f,
                    textPaint,
                )
                if (node.childCount > 0) {
                    drawContext.canvas.nativeCanvas.drawText(
                        "子节点 ${node.childCount}",
                        position.x + 16f,
                        position.y + 68f,
                        textPaint.apply { color = labelColor.copy(alpha = 0.6f).toArgb() },
                    )
                }
            }
        }
    }
}

private fun Color.toArgb(): Int = android.graphics.Color.argb(
    (alpha * 255).toInt(),
    (red * 255).toInt(),
    (green * 255).toInt(),
    (blue * 255).toInt(),
)
