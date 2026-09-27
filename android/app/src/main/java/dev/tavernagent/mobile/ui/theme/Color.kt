package dev.tavernagent.mobile.ui.theme

import androidx.compose.material3.ColorScheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color

// 值来自 web/src/styles/tokens.css（唯一权威来源）。改这里必须同步 Web 端令牌。

private fun lightScheme(): ColorScheme = lightColorScheme(
    primary = Color(0xFF2563EB),
    onPrimary = Color(0xFFFFFFFF),
    primaryContainer = Color(0xFFEFF6FF),
    onPrimaryContainer = Color(0xFF1D4ED8),
    secondary = Color(0xFF059669),
    onSecondary = Color(0xFFFFFFFF),
    secondaryContainer = Color(0xFFE7F6F1),
    onSecondaryContainer = Color(0xFF065F46),
    tertiary = Color(0xFFD97706),
    onTertiary = Color(0xFFFFFFFF),
    tertiaryContainer = Color(0xFFFDF3E3),
    onTertiaryContainer = Color(0xFF92400E),
    error = Color(0xFFDC2626),
    onError = Color(0xFFFFFFFF),
    errorContainer = Color(0xFFFDECEC),
    onErrorContainer = Color(0xFF991B1B),
    background = Color(0xFFFAFAFC),
    onBackground = Color(0xFF111827),
    surface = Color(0xFFFFFFFF),
    onSurface = Color(0xFF111827),
    surfaceVariant = Color(0xFFF3F4F6),
    onSurfaceVariant = Color(0xFF4B5563),
    surfaceContainerLowest = Color(0xFFFFFFFF),
    surfaceContainerLow = Color(0xFFF8F9FA),
    surfaceContainer = Color(0xFFF3F4F6),
    surfaceContainerHigh = Color(0xFFFFFFFF),
    surfaceContainerHighest = Color(0xFFFFFFFF),
    outline = Color(0xFFD1D5DB),
    outlineVariant = Color(0xFFE5E7EB),
    inverseSurface = Color(0xFF111827),
    inverseOnSurface = Color(0xFFF4F4F6),
    scrim = Color(0x730F1729),
)

private fun darkScheme(): ColorScheme = darkColorScheme(
    primary = Color(0xFF3B82F6),
    onPrimary = Color(0xFFFFFFFF),
    primaryContainer = Color(0xFF1D4ED8),
    onPrimaryContainer = Color(0xFFEFF6FF),
    secondary = Color(0xFF10B981),
    onSecondary = Color(0xFF06281D),
    secondaryContainer = Color(0xFF132F24),
    onSecondaryContainer = Color(0xFFA7F3D0),
    tertiary = Color(0xFFF59E0B),
    onTertiary = Color(0xFF2A1B02),
    tertiaryContainer = Color(0xFF3A2A0D),
    onTertiaryContainer = Color(0xFFFDE68A),
    error = Color(0xFFEF4444),
    onError = Color(0xFF2A0A0A),
    errorContainer = Color(0xFF382124),
    onErrorContainer = Color(0xFFFCA5A5),
    background = Color(0xFF101114),
    onBackground = Color(0xFFF4F4F6),
    surface = Color(0xFF1A1B1F),
    onSurface = Color(0xFFF4F4F6),
    surfaceVariant = Color(0xFF222329),
    onSurfaceVariant = Color(0xFF9FA0A9),
    surfaceContainerLowest = Color(0xFF0B0C0E),
    surfaceContainerLow = Color(0xFF141518),
    surfaceContainer = Color(0xFF1A1B1F),
    surfaceContainerHigh = Color(0xFF222329),
    surfaceContainerHighest = Color(0xFF28292F),
    outline = Color(0xFF383942),
    outlineVariant = Color(0xFF28292F),
    inverseSurface = Color(0xFFF4F4F6),
    inverseOnSurface = Color(0xFF101114),
    scrim = Color(0xA6000000),
)

private fun violetScheme(): ColorScheme = darkColorScheme(
    primary = Color(0xFF8B5CF6),
    onPrimary = Color(0xFFFFFFFF),
    primaryContainer = Color(0xFF6D28D9),
    onPrimaryContainer = Color(0xFFEDE9FE),
    secondary = Color(0xFF10B981),
    onSecondary = Color(0xFF042F20),
    secondaryContainer = Color(0xFF0D3D2C),
    onSecondaryContainer = Color(0xFFA7F3D0),
    tertiary = Color(0xFFF59E0B),
    onTertiary = Color(0xFF2A1B02),
    tertiaryContainer = Color(0xFF3F2B07),
    onTertiaryContainer = Color(0xFFFDE68A),
    error = Color(0xFFF43F5E),
    onError = Color(0xFF2F060E),
    errorContainer = Color(0xFF4C0E1C),
    onErrorContainer = Color(0xFFFECDD3),
    background = Color(0xFF13111A),
    onBackground = Color(0xFFF3F0F9),
    surface = Color(0xFF1D1927),
    onSurface = Color(0xFFF3F0F9),
    surfaceVariant = Color(0xFF252033),
    onSurfaceVariant = Color(0xFF9D94AB),
    surfaceContainerLowest = Color(0xFF0F0D15),
    surfaceContainerLow = Color(0xFF161420),
    surfaceContainer = Color(0xFF1D1927),
    surfaceContainerHigh = Color(0xFF252033),
    surfaceContainerHighest = Color(0xFF2D263B),
    outline = Color(0xFF3D3350),
    outlineVariant = Color(0xFF2D263B),
    inverseSurface = Color(0xFFF3F0F9),
    inverseOnSurface = Color(0xFF13111A),
    scrim = Color(0xAE08060C),
)

val TavernLightScheme: ColorScheme = lightScheme()
val TavernDarkScheme: ColorScheme = darkScheme()
val TavernVioletScheme: ColorScheme = violetScheme()

/**
 * M3 配色之外的语义色（量表、记忆分类、意图徽标、状态底）。
 * 与 Web 端令牌一一对应；暗色主题下部分令牌沿用 light 值（与 Web 行为一致）。
 */
data class TavernColors(
    val meterAffection: Color = Color(0xFFE11D48),
    val meterTrust: Color = Color(0xFF2563EB),
    val meterAlert: Color = Color(0xFFD97706),
    val success: Color = Color(0xFF059669),
    val successBg: Color = Color(0x17059669),
    val warning: Color = Color(0xFFD97706),
    val warningBg: Color = Color(0x14D97706),
    val info: Color = Color(0xFF2563EB),
    val dangerText: Color = Color(0xFFDC2626),
    val dangerBg: Color = Color(0x14DC2626),
    val memoryEpisodic: Color = Color(0xFF2563EB),
    val memoryInference: Color = Color(0xFFD97706),
    val memorySecret: Color = Color(0xFF9333EA),
    val intentAggressive: Color = Color(0xFFDC2626),
    val intentClever: Color = Color(0xFF2563EB),
    val intentEmotional: Color = Color(0xFF059669),
    val intentChaotic: Color = Color(0xFFD97706),
    val borderHairline: Color = Color(0xFFE5E7EB),
)

val tavernVioletColors = TavernColors(
    meterAffection = Color(0xFFF43F5E),
    meterTrust = Color(0xFF8B5CF6),
    meterAlert = Color(0xFFF59E0B),
    success = Color(0xFF10B981),
    successBg = Color(0x2410B981),
    warning = Color(0xFFF59E0B),
    warningBg = Color(0x24F59E0B),
    info = Color(0xFF8B5CF6),
    dangerText = Color(0xFFFB7185),
    dangerBg = Color(0x24F43F5E),
    memoryEpisodic = Color(0xFF8B5CF6),
    memoryInference = Color(0xFFF59E0B),
    memorySecret = Color(0xFFC084FC),
    intentAggressive = Color(0xFFFB7185),
    intentClever = Color(0xFF8B5CF6),
    intentEmotional = Color(0xFF10B981),
    intentChaotic = Color(0xFFF59E0B),
    borderHairline = Color(0xFF2D263B),
)

fun tavernColorsFor(dark: Boolean): TavernColors = if (dark) {
    TavernColors(
        meterAffection = Color(0xFFF43F5E),
        meterTrust = Color(0xFF3B82F6),
        meterAlert = Color(0xFFF59E0B),
        success = Color(0xFF10B981),
        successBg = Color(0x2410B981),
        warning = Color(0xFFF59E0B),
        warningBg = Color(0x24F59E0B),
        info = Color(0xFF3B82F6),
        dangerText = Color(0xFFF87171),
        dangerBg = Color(0x24EF4444),
        memoryEpisodic = Color(0xFF3B82F6),
        memoryInference = Color(0xFFF59E0B),
        memorySecret = Color(0xFFA855F7),
        intentAggressive = Color(0xFFF87171),
        intentClever = Color(0xFF3B82F6),
        intentEmotional = Color(0xFF10B981),
        intentChaotic = Color(0xFFF59E0B),
        borderHairline = Color(0xFF28292F),
    )
} else {
    TavernColors()
}

fun tavernColorsFor(
    themeMode: dev.tavernagent.mobile.data.local.ThemeMode,
    isSystemDark: Boolean = false,
): TavernColors = when (themeMode) {
    dev.tavernagent.mobile.data.local.ThemeMode.LIGHT -> tavernColorsFor(false)
    dev.tavernagent.mobile.data.local.ThemeMode.DARK -> tavernColorsFor(true)
    dev.tavernagent.mobile.data.local.ThemeMode.VIOLET -> tavernVioletColors
    dev.tavernagent.mobile.data.local.ThemeMode.SYSTEM -> tavernColorsFor(isSystemDark)
}

val LocalTavernColors = staticCompositionLocalOf { TavernColors() }
