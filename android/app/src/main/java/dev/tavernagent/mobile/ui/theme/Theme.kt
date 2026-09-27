package dev.tavernagent.mobile.ui.theme

import android.app.Activity
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.SideEffect
import androidx.compose.ui.platform.LocalView
import androidx.core.view.WindowCompat
import dev.tavernagent.mobile.data.local.ThemeMode

@Composable
fun TavernAgentTheme(
    themeMode: ThemeMode = ThemeMode.SYSTEM,
    darkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit,
) {
    val isDark = when (themeMode) {
        ThemeMode.LIGHT -> false
        ThemeMode.DARK,
        ThemeMode.VIOLET -> true
        ThemeMode.SYSTEM -> darkTheme
    }
    val colorScheme = when (themeMode) {
        ThemeMode.LIGHT -> TavernLightScheme
        ThemeMode.DARK -> TavernDarkScheme
        ThemeMode.VIOLET -> TavernVioletScheme
        ThemeMode.SYSTEM -> if (darkTheme) TavernDarkScheme else TavernLightScheme
    }
    val tavernColors = tavernColorsFor(themeMode, darkTheme)

    val view = LocalView.current
    if (!view.isInEditMode) {
        SideEffect {
            val window = (view.context as? Activity)?.window ?: return@SideEffect
            WindowCompat.getInsetsController(window, view).isAppearanceLightStatusBars = !isDark
            WindowCompat.getInsetsController(window, view).isAppearanceLightNavigationBars = !isDark
        }
    }

    MaterialTheme(
        colorScheme = colorScheme,
        shapes = TavernShapes,
        typography = TavernTypography,
        content = {
            CompositionLocalProvider(
                LocalTavernDimens provides TavernDimens(),
                LocalTavernColors provides tavernColors,
                LocalTavernTextStyles provides TavernTextStyles(),
                content = content,
            )
        },
    )
}
