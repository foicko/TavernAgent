package dev.tavernagent.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.BrokenImage
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import coil.compose.SubcomposeAsyncImage
import coil.request.ImageRequest
import kotlinx.coroutines.delay

/**
 * 会话正文里的卡内插画 / 立绘。
 *
 * 三种状态都有明确落点，避免「什么都不显示」的沉默失败：
 * - 加载中（含资源表尚未就绪）：固定高度占位 + 细进度圈，列表不会跳动；
 * - 加载失败：点按重试（用 URL 片段换缓存键重发）；
 * - 资源不存在（资源表就绪后仍查无此键）：提示缺失键名，不发必然 404 的请求。
 *
 * 失败 / 缺失提示都有 500ms 宽限期：资源表是异步到达的，短暂的解析失败不应闪出错误。
 */
@Composable
fun CardImage(
    url: String,
    rawKey: String,
    onOpen: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val shape = RoundedCornerShape(12.dp)
    var attempt by remember(url) { mutableIntStateOf(0) }
    var graceElapsed by remember(url) { mutableStateOf(false) }
    LaunchedEffect(url) {
        graceElapsed = false
        delay(500)
        graceElapsed = true
    }

    if (url.isBlank()) {
        if (graceElapsed) {
            MissingImage(
                rawKey = rawKey,
                reason = "未找到对应资源",
                modifier = modifier,
            )
        } else {
            LoadingBox(
                modifier = modifier
                    .clip(shape)
                    .border(1.dp, MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.5f), shape),
            )
        }
        return
    }

    val context = LocalContext.current
    SubcomposeAsyncImage(
        model = ImageRequest.Builder(context)
            // 片段只用于换缓存键重试：不参与网络请求，也不影响内存缓存的大小维度。
            .data(if (attempt == 0) url else "$url#ta-retry=$attempt")
            .build(),
        contentDescription = rawKey.ifBlank { "插画立绘" },
        contentScale = ContentScale.Fit,
        modifier = modifier
            .fillMaxWidth()
            .heightIn(min = 120.dp, max = 380.dp)
            .clip(shape)
            .border(1.dp, MaterialTheme.colorScheme.outlineVariant, shape)
            .clickable { onOpen(url) },
        loading = { LoadingBox() },
        error = {
            if (graceElapsed) {
                MissingImage(
                    rawKey = rawKey,
                    reason = "图片加载失败 · 点按重试",
                    modifier = Modifier.clickable { attempt++ },
                )
            } else {
                LoadingBox()
            }
        },
    )
}

@Composable
private fun LoadingBox(modifier: Modifier = Modifier) {
    Box(
        modifier = modifier
            .fillMaxWidth()
            .height(180.dp),
        contentAlignment = Alignment.Center,
    ) {
        CircularProgressIndicator(modifier = Modifier.size(22.dp), strokeWidth = 2.dp)
    }
}

/** 缺失 / 失败占位：紧凑、克制，但把原因与键名说清楚。 */
@Composable
private fun MissingImage(
    rawKey: String,
    reason: String,
    modifier: Modifier = Modifier,
) {
    val shape = RoundedCornerShape(12.dp)
    Box(
        modifier = modifier
            .fillMaxWidth()
            .heightIn(min = 84.dp)
            .clip(shape)
            .background(MaterialTheme.colorScheme.surfaceContainerLowest)
            .border(1.dp, MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.7f), shape),
        contentAlignment = Alignment.Center,
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier.padding(horizontal = 14.dp, vertical = 12.dp),
        ) {
            Icon(
                Icons.Filled.BrokenImage,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.7f),
                modifier = Modifier.size(18.dp),
            )
            Column(modifier = Modifier.padding(start = 10.dp)) {
                Text(
                    text = reason,
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                if (rawKey.isNotBlank()) {
                    Text(
                        text = rawKey,
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.65f),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
        }
    }
}
