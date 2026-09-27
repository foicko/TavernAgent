package dev.tavernagent.mobile.ui.screens.story

import androidx.compose.foundation.background
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
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Bookmark
import androidx.compose.material.icons.filled.Handshake
import androidx.compose.material.icons.filled.Inventory2
import androidx.compose.material.icons.filled.Person
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.PrimaryTabRow
import androidx.compose.material3.SheetState
import androidx.compose.material3.Surface
import androidx.compose.material3.Tab
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import dev.tavernagent.mobile.data.remote.dto.SessionViewDto
import dev.tavernagent.mobile.data.remote.dto.WorldStateDto
import dev.tavernagent.mobile.ui.components.MeterBar
import dev.tavernagent.mobile.ui.theme.LocalTavernColors
import dev.tavernagent.mobile.ui.theme.dimens

/**
 * 沉浸式世界线状态与角色属性抽屉（World State Sheet）：
 * 允许玩家在对话流中直接上滑或点击头像唤出，展示人物好感/信任/警觉量表、物品背包与约定承诺。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun WorldStateSheet(
    view: SessionViewDto?,
    world: WorldStateDto?,
    sheetState: SheetState,
    onDismiss: () -> Unit,
    onOpenFullDetail: () -> Unit,
) {
    val colors = LocalTavernColors.current
    var selectedTab by remember { mutableIntStateOf(0) }
    val tabs = listOf("角色好感", "物品清单", "约定承诺")

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = sheetState,
        containerColor = MaterialTheme.colorScheme.surfaceContainerLow,
        shape = RoundedCornerShape(topStart = 24.dp, topEnd = 24.dp),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(max = 520.dp)
                .padding(bottom = dimens.space4),
        ) {
            // 顶栏标题与查看全屏详情链接
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = dimens.space4, vertical = dimens.space2),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    text = "世界线状态与属性",
                    style = MaterialTheme.typography.titleMedium,
                    fontWeight = FontWeight.Bold,
                )
                TextButton(onClick = {
                    onDismiss()
                    onOpenFullDetail()
                }) {
                    Text("完整状态面板 →")
                }
            }

            // 分类 Tab
            PrimaryTabRow(
                selectedTabIndex = selectedTab,
                containerColor = Color.Transparent,
            ) {
                tabs.forEachIndexed { index, title ->
                    Tab(
                        selected = selectedTab == index,
                        onClick = { selectedTab = index },
                        text = {
                            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                                val icon = when (index) {
                                    0 -> Icons.Filled.Person
                                    1 -> Icons.Filled.Inventory2
                                    else -> Icons.Filled.Handshake
                                }
                                Icon(icon, contentDescription = null, modifier = Modifier.size(14.dp))
                                Text(title, style = MaterialTheme.typography.labelMedium)
                            }
                        },
                    )
                }
            }

            Spacer(Modifier.height(dimens.space2))

            Box(modifier = Modifier.fillMaxWidth().weight(1f).padding(horizontal = dimens.space4)) {
                when (selectedTab) {
                    0 -> CharacterAffectionTab(world = world)
                    1 -> ItemsListTab(world = world)
                    2 -> PromisesListTab(world = world)
                }
            }
        }
    }
}

@Composable
private fun CharacterAffectionTab(world: WorldStateDto?) {
    val colors = LocalTavernColors.current
    val characters = world?.characters.orEmpty().values.filter { it.characterId != "player" }

    if (characters.isEmpty()) {
        Box(modifier = Modifier.fillMaxWidth().padding(dimens.space4), contentAlignment = Alignment.Center) {
            Text("当前分支中尚未记录同伴档案", color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        return
    }

    LazyColumn(verticalArrangement = Arrangement.spacedBy(dimens.space3)) {
        items(characters, key = { it.characterId }) { character ->
            val relation = world?.relationships?.get(character.characterId)
            val mood = world?.moods?.get(character.characterId)

            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh),
                shape = RoundedCornerShape(16.dp),
            ) {
                Column(
                    modifier = Modifier.padding(dimens.space3),
                    verticalArrangement = Arrangement.spacedBy(dimens.space2),
                ) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Text(
                            text = character.name.ifBlank { character.characterId },
                            style = MaterialTheme.typography.titleSmall,
                            fontWeight = FontWeight.Bold,
                        )
                        if (character.participant) {
                            Surface(
                                color = MaterialTheme.colorScheme.primaryContainer,
                                shape = RoundedCornerShape(4.dp),
                            ) {
                                Text(
                                    text = "核心主角",
                                    style = MaterialTheme.typography.labelSmall,
                                    color = MaterialTheme.colorScheme.onPrimaryContainer,
                                    modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp),
                                )
                            }
                        }
                    }

                    mood?.let {
                        Text(
                            text = "心境：${it.text.ifBlank { it.moodCode }}",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }

                    if (relation != null) {
                        Column(verticalArrangement = Arrangement.spacedBy(dimens.space1)) {
                            MeterBar("好感度", relation.affection, colors.meterAffection)
                            MeterBar("信任度", relation.trust, colors.meterTrust)
                            MeterBar("警觉度", relation.alertness, colors.meterAlert)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun ItemsListTab(world: WorldStateDto?) {
    val items = world?.items.orEmpty().values.toList()

    if (items.isEmpty()) {
        Box(modifier = Modifier.fillMaxWidth().padding(dimens.space4), contentAlignment = Alignment.Center) {
            Text("身上暂无任何道具或信物", color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        return
    }

    LazyColumn(verticalArrangement = Arrangement.spacedBy(dimens.space2)) {
        items(items, key = { it.instanceId }) { item ->
            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh),
                shape = RoundedCornerShape(12.dp),
            ) {
                Row(
                    modifier = Modifier.padding(dimens.space3),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween,
                ) {
                    Column(modifier = Modifier.weight(1f)) {
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(dimens.space1)) {
                            Text(text = item.name, style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.SemiBold)
                            if (item.keepsake) {
                                Surface(
                                    color = MaterialTheme.colorScheme.tertiaryContainer,
                                    shape = RoundedCornerShape(4.dp),
                                ) {
                                    Text(
                                        text = "信物",
                                        style = MaterialTheme.typography.labelSmall,
                                        color = MaterialTheme.colorScheme.onTertiaryContainer,
                                        modifier = Modifier.padding(horizontal = 4.dp, vertical = 1.dp),
                                    )
                                }
                            }
                        }
                        if (item.aliases.isNotEmpty()) {
                            Text(
                                text = "别名：${item.aliases.joinToString(" / ")}",
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                    Text(
                        text = "×${item.quantity}",
                        style = MaterialTheme.typography.titleSmall,
                        color = MaterialTheme.colorScheme.primary,
                        fontWeight = FontWeight.Bold,
                    )
                }
            }
        }
    }
}

@Composable
private fun PromisesListTab(world: WorldStateDto?) {
    val promises = world?.promises.orEmpty().values.toList()

    if (promises.isEmpty()) {
        Box(modifier = Modifier.fillMaxWidth().padding(dimens.space4), contentAlignment = Alignment.Center) {
            Text("当前世界线尚未立下明确的契约或承诺", color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        return
    }

    LazyColumn(verticalArrangement = Arrangement.spacedBy(dimens.space2)) {
        items(promises, key = { it.promiseId }) { promise ->
            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh),
                shape = RoundedCornerShape(12.dp),
            ) {
                Column(modifier = Modifier.padding(dimens.space3), verticalArrangement = Arrangement.spacedBy(dimens.space1)) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Text(
                            text = promise.content,
                            style = MaterialTheme.typography.bodyMedium,
                            fontWeight = FontWeight.Medium,
                            modifier = Modifier.weight(1f),
                        )
                        val (label, bg) = when (promise.state) {
                            "fulfilled" -> "已践行" to MaterialTheme.colorScheme.secondaryContainer
                            "broken" -> "已破裂" to MaterialTheme.colorScheme.errorContainer
                            else -> "信守中" to MaterialTheme.colorScheme.primaryContainer
                        }
                        Surface(color = bg, shape = RoundedCornerShape(4.dp)) {
                            Text(
                                text = label,
                                style = MaterialTheme.typography.labelSmall,
                                modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp),
                            )
                        }
                    }
                }
            }
        }
    }
}
