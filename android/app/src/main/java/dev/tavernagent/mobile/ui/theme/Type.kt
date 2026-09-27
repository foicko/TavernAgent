package dev.tavernagent.mobile.ui.theme

import androidx.compose.material3.Typography
import androidx.compose.runtime.Composable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp

/** UI 字体族：默认 sans（Inter 在 Web；Android 用系统 sans）。 */
val TavernTypography = Typography()

/** 叙事专用的额外文本样式（Web tokens：--font-narrative / --font-mono）。 */
data class TavernTextStyles(
    val narrative: TextStyle = TextStyle(
        fontFamily = FontFamily.Serif,
        fontSize = 15.5.sp,
        lineHeight = 28.sp,
    ),
    val narration: TextStyle = TextStyle(
        fontFamily = FontFamily.Serif,
        fontSize = 15.5.sp,
        lineHeight = 28.5.sp,
    ),
    val dialogue: TextStyle = TextStyle(
        fontFamily = FontFamily.Serif,
        fontSize = 15.sp,
        lineHeight = 27.sp,
    ),
    val innerMonologue: TextStyle = TextStyle(
        fontFamily = FontFamily.Serif,
        fontSize = 14.5.sp,
        lineHeight = 26.sp,
    ),
    val playerSpeech: TextStyle = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontSize = 15.sp,
        lineHeight = 24.sp,
    ),
    val thinking: TextStyle = TextStyle(
        fontFamily = FontFamily.Monospace,
        fontSize = 12.sp,
        lineHeight = 20.sp,
    ),
    val meta: TextStyle = TextStyle(
        fontFamily = FontFamily.SansSerif,
        fontSize = 11.5.sp,
        lineHeight = 16.sp,
        fontWeight = FontWeight.Medium,
    ),
)

val LocalTavernTextStyles = androidx.compose.runtime.staticCompositionLocalOf { TavernTextStyles() }

val texts: TavernTextStyles
    @Composable
    @ReadOnlyComposable
    get() = LocalTavernTextStyles.current
