package dev.tavernagent.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil.compose.SubcomposeAsyncImage
import kotlin.math.abs

/**
 * 角色与玩家专属头像组件：
 * - 优先加载网络 / 本地 / Base64 图片（经 Coil 渲染）
 * - 加载中或失败时自动降级展示依据名称 Hash 派生的彩色渐变首字符徽标
 * - 支持圆角裁切与外发丝线
 */
@Composable
fun StoryAvatar(
    name: String,
    avatarUrl: String?,
    modifier: Modifier = Modifier,
    size: Dp = 40.dp,
    shape: androidx.compose.ui.graphics.Shape = RoundedCornerShape(12.dp),
    borderColor: Color = MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.6f),
    onClick: (() -> Unit)? = null,
) {
    val clickableModifier = if (onClick != null) {
        modifier.clickable(onClick = onClick)
    } else {
        modifier
    }

    Box(
        modifier = clickableModifier
            .size(size)
            .clip(shape)
            .border(1.dp, borderColor, shape),
        contentAlignment = Alignment.Center,
    ) {
        if (!avatarUrl.isNullOrBlank()) {
            SubcomposeAsyncImage(
                model = avatarUrl,
                contentDescription = "$name 头像",
                contentScale = ContentScale.Crop,
                modifier = Modifier.matchParentSize(),
                loading = {
                    InitialPlaceholder(name = name)
                },
                error = {
                    InitialPlaceholder(name = name)
                },
            )
        } else {
            InitialPlaceholder(name = name)
        }
    }
}

@Composable
private fun InitialPlaceholder(name: String) {
    val initial = name.trim().take(1).uppercase().ifBlank { "?" }
    val gradient = deterministicGradient(name)

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(brush = gradient),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            text = initial,
            color = Color.White,
            fontWeight = FontWeight.Bold,
            fontSize = 16.sp,
        )
    }
}

/** 根据名称哈希生成确定性的鲜明渐变色彩，避免头像缺失时的千篇一律 */
private fun deterministicGradient(name: String): Brush {
    val hash = abs(name.hashCode())
    val palettes = listOf(
        listOf(Color(0xFF3B82F6), Color(0xFF1D4ED8)), // 蓝
        listOf(Color(0xFF8B5CF6), Color(0xFF6D28D9)), // 紫
        listOf(Color(0xFFEC4899), Color(0xFFBE185D)), // 粉
        listOf(Color(0xFFF59E0B), Color(0xFFD97706)), // 琥珀
        listOf(Color(0xFF10B981), Color(0xFF047857)), // 翡翠
        listOf(Color(0xFF06B6D4), Color(0xFF0E7490)), // 青
        listOf(Color(0xFF6366F1), Color(0xFF4338CA)), // 靛
        listOf(Color(0xFFEF4444), Color(0xFFB91C1C)), // 绯红
    )
    val pair = palettes[hash % palettes.size]
    return Brush.linearGradient(pair)
}
