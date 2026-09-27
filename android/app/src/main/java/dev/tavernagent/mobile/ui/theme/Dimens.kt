package dev.tavernagent.mobile.ui.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

/**
 * 与 Web 端一致的间距刻度（8dp 网格，含 4dp 半步）。
 * 组件间距统一从这里取，避免散落的字面值。
 */
data class TavernDimens(
    val space1: Dp = 4.dp,
    val space2: Dp = 8.dp,
    val space3: Dp = 12.dp,
    val space4: Dp = 16.dp,
    val space5: Dp = 24.dp,
    val space6: Dp = 32.dp,
    val readingMeasure: Dp = 720.dp,
    val panelWidth: Dp = 360.dp,
)

val LocalTavernDimens = staticCompositionLocalOf { TavernDimens() }

/** 全局间距入口：`dimens.space4`。在 [TavernAgentTheme] 中提供。 */
val dimens: TavernDimens
    @Composable
    @ReadOnlyComposable
    get() = LocalTavernDimens.current
