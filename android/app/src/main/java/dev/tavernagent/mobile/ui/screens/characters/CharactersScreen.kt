package dev.tavernagent.mobile.ui.screens.characters

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
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
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Tab
import androidx.compose.material3.TabRow
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.tavernagent.mobile.data.remote.dto.ItemInstanceDto
import dev.tavernagent.mobile.data.remote.dto.PromiseDto
import dev.tavernagent.mobile.data.remote.dto.SessionViewDto
import dev.tavernagent.mobile.data.remote.dto.WorldStateDto
import dev.tavernagent.mobile.ui.components.ErrorCard
import dev.tavernagent.mobile.ui.components.LoadingBox
import dev.tavernagent.mobile.ui.components.MeterBar
import dev.tavernagent.mobile.ui.components.SectionHeader
import dev.tavernagent.mobile.ui.theme.LocalTavernColors
import dev.tavernagent.mobile.ui.theme.dimens

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CharactersScreen(
    sessionId: String,
    branchId: String,
    onBack: () -> Unit,
) {
    val viewModel: CharactersViewModel = viewModel(factory = CharactersViewModel.factory())
    val state by viewModel.state.collectAsStateWithLifecycle()
    var tab by remember { mutableIntStateOf(0) }
    val tabs = listOf("角色", "物品", "承诺", "世界")

    LaunchedEffect(sessionId, branchId) { viewModel.load(sessionId, branchId) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("角色与状态") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回")
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
                    message = state.error ?: "无法加载状态",
                    modifier = Modifier.padding(dimens.space4),
                    onRetry = { viewModel.load(sessionId, branchId) },
                )
                else -> Column {
                    TabRow(selectedTabIndex = tab) {
                        tabs.forEachIndexed { index, label ->
                            Tab(selected = tab == index, onClick = { tab = index }, text = { Text(label) })
                        }
                    }
                    when (tab) {
                        0 -> CharactersTab(view = view, world = state.world)
                        1 -> ItemsTab(world = state.world)
                        2 -> PromisesTab(world = state.world, view = view)
                        else -> WorldTab(view = view)
                    }
                }
            }
        }
    }
}

@Composable
private fun CharactersTab(view: SessionViewDto, world: WorldStateDto?) {
    val colors = LocalTavernColors.current
    val characters = world?.characters.orEmpty().values.toList()
    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(dimens.space4),
        verticalArrangement = Arrangement.spacedBy(dimens.space3),
    ) {
        if (characters.isEmpty()) {
            item { Text("状态里还没有角色档案。", color = MaterialTheme.colorScheme.onSurfaceVariant) }
        }
        items(characters, key = { it.characterId }) { character ->
            val relation = world?.relationships?.get(character.characterId)
            val mood = world?.moods?.get(character.characterId)
            val goals = world?.goals?.values?.filter { it.characterId == character.characterId }.orEmpty()
            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh),
            ) {
                Column(
                    modifier = Modifier.padding(dimens.space4),
                    verticalArrangement = Arrangement.spacedBy(dimens.space3),
                ) {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(dimens.space2)) {
                        Text(
                            character.name.ifBlank { character.characterId },
                            style = MaterialTheme.typography.titleMedium,
                            fontWeight = FontWeight.SemiBold,
                        )
                        if (character.participant) {
                            Pill("主要角色", MaterialTheme.colorScheme.primary)
                        }
                    }
                    if (character.aliases.isNotEmpty()) {
                        Text(
                            "别称：${character.aliases.joinToString("、")}",
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    mood?.let {
                        Text(
                            text = "心境：${it.text.ifBlank { it.moodCode }}",
                            style = MaterialTheme.typography.bodyMedium,
                        )
                    }
                    if (character.description.isNotBlank()) {
                        Text(
                            character.description.take(240),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    relation?.let { value ->
                        Column(verticalArrangement = Arrangement.spacedBy(dimens.space2)) {
                            MeterBar("好感", value.affection, colors.meterAffection)
                            MeterBar("信任", value.trust, colors.meterTrust)
                            MeterBar("警觉", value.alertness, colors.meterAlert)
                        }
                    }
                    if (goals.isNotEmpty()) {
                        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                            Text("目标", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            goals.forEach { goal -> Text("· ${goal.text}", style = MaterialTheme.typography.bodySmall) }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun ItemsTab(world: WorldStateDto?) {
    val items = world?.items.orEmpty().values
        .filter { it.quantity > 0 }
        .sortedWith(compareByDescending<ItemInstanceDto> { it.ownerId == "player" }.thenBy { it.name })
    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(dimens.space4),
        verticalArrangement = Arrangement.spacedBy(dimens.space2),
    ) {
        if (items.isEmpty()) {
            item { Text("身上和世界里都没有物品记录。", color = MaterialTheme.colorScheme.onSurfaceVariant) }
        }
        items(items, key = { it.instanceId }) { item ->
            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh),
            ) {
                Row(
                    modifier = Modifier.padding(dimens.space3),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Column(modifier = Modifier.weight(1f)) {
                        Text(item.name.ifBlank { item.templateId }, style = MaterialTheme.typography.bodyLarge)
                        Text(
                            text = when {
                                item.ownerId == "player" -> "你持有"
                                item.ownerId.isNullOrBlank() -> item.locationId?.let { "位于 $it" } ?: "无主"
                                else -> "持有者：${item.ownerId}"
                            },
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    if (item.quantity > 1) Pill("×${item.quantity}", MaterialTheme.colorScheme.secondary)
                    if (item.keepsake) Pill("信物", MaterialTheme.colorScheme.tertiary)
                    if (item.protection > 0) Pill("保护 ${item.protection}", MaterialTheme.colorScheme.outline)
                }
            }
        }
    }
}

@Composable
private fun PromisesTab(world: WorldStateDto?, view: SessionViewDto) {
    val promises = world?.promises.orEmpty().values.toList()
    val names = world?.characters.orEmpty()
    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(dimens.space4),
        verticalArrangement = Arrangement.spacedBy(dimens.space2),
    ) {
        if (promises.isEmpty()) {
            item { Text("还没有立下过承诺。", color = MaterialTheme.colorScheme.onSurfaceVariant) }
        }
        items(promises, key = { it.promiseId }) { promise ->
            PromiseRow(promise, names.mapValues { it.value.name })
        }
        if (view.secrets.isNotEmpty()) {
            item { SectionHeader("秘密") }
            items(view.secrets, key = { it.secretId }) { secret ->
                Card(
                    modifier = Modifier.fillMaxWidth(),
                    colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh),
                ) {
                    Column(modifier = Modifier.padding(dimens.space3), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                        Row(horizontalArrangement = Arrangement.spacedBy(dimens.space2), verticalAlignment = Alignment.CenterVertically) {
                            Text(secret.title ?: "未命名秘密", style = MaterialTheme.typography.bodyLarge)
                            Pill(if (secret.revealed) "已揭示" else "未揭示", if (secret.revealed) MaterialTheme.colorScheme.secondary else MaterialTheme.colorScheme.outline)
                        }
                        if (secret.revealed && !secret.content.isNullOrBlank()) {
                            Text(secret.content, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun PromiseRow(promise: PromiseDto, names: Map<String, String>) {
    val color = when (promise.state) {
        "active" -> MaterialTheme.colorScheme.primary
        "fulfilled" -> MaterialTheme.colorScheme.secondary
        "broken" -> MaterialTheme.colorScheme.error
        else -> MaterialTheme.colorScheme.outline
    }
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh),
    ) {
        Column(modifier = Modifier.padding(dimens.space3), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Row(horizontalArrangement = Arrangement.spacedBy(dimens.space2), verticalAlignment = Alignment.CenterVertically) {
                Pill(stateLabel(promise.state), color)
                Text(
                    promise.participantIds.joinToString("、") { names[it] ?: it },
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Text(promise.content, style = MaterialTheme.typography.bodyMedium)
        }
    }
}

private fun stateLabel(state: String): String = when (state) {
    "proposed" -> "提议中"
    "active" -> "生效中"
    "fulfilled" -> "已兑现"
    "broken" -> "已违背"
    "cancelled" -> "已取消"
    else -> state
}

@Composable
private fun WorldTab(view: SessionViewDto) {
    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(dimens.space4),
        verticalArrangement = Arrangement.spacedBy(dimens.space2),
    ) {
        item { SectionHeader("剧情大纲") }
        view.outline?.scene?.takeIf { it.isNotBlank() }?.let { scene ->
            item {
                Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh)) {
                    Text("当前场景：$scene", modifier = Modifier.padding(dimens.space3))
                }
            }
        }
        items(view.outline?.milestones.orEmpty(), key = { it.milestoneId }) { milestone ->
            Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh)) {
                Text("· ${milestone.description}", modifier = Modifier.padding(dimens.space3))
            }
        }
        if (view.outline?.milestones.isNullOrEmpty()) {
            item { Text("还没有里程碑。", color = MaterialTheme.colorScheme.onSurfaceVariant) }
        }
        item { SectionHeader("前情摘要") }
        view.activeSummary?.text?.takeIf { it.isNotBlank() }?.let { summary ->
            item {
                Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow)) {
                    Text(summary, modifier = Modifier.padding(dimens.space3), style = MaterialTheme.typography.bodySmall)
                }
            }
        } ?: item { Text("尚无可用的交接摘要。", color = MaterialTheme.colorScheme.onSurfaceVariant) }
    }
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
