using System.Runtime.InteropServices;
using Avalonia;
using Avalonia.Markup.Xaml;
using Avalonia.Styling;

namespace TavernAgent.Desktop.Services;

/// <summary>
/// Respects the OS "reduce motion" accessibility preference. Windows exposes it via
/// SPI_GETCLIENTAREAANIMATION; other platforms default to full motion. Override with
/// the TAVERNAGENT_REDUCE_MOTION=1 environment variable (also handy for tests).
/// </summary>
public static class MotionPreference
{
    private const uint SpiGetClientAreaAnimation = 0x1042;
    private const string MotionStyleUri = "avares://TavernAgent.Desktop/Styles/Motion.axaml";
    private static bool _applied;

    public static bool ReduceMotion { get; private set; }

    public static void Apply()
    {
        if (_applied) return;
        _applied = true;
        ReduceMotion = Detect();
        if (Application.Current is not { } app) return;
        try
        {
            // Motion.axaml drives everything through Style.Animations. Those cannot be
            // neutralised by a later setter (unlike Transitions), so the whole layer is
            // simply not loaded when the user asked for reduced motion.
            var uri = new Uri(ReduceMotion
                ? "avares://TavernAgent.Desktop/Styles/ReducedMotion.axaml"
                : MotionStyleUri);
            if (AvaloniaXamlLoader.Load(uri) is not Styles styles) return;

            if (ReduceMotion) app.Styles.Add(styles);
            else app.Styles.Insert(1, styles); // keep the original slot: after FluentTheme
        }
        catch
        {
            // Never let an a11y nicety break startup.
        }
    }

    private static bool Detect()
    {
        var env = Environment.GetEnvironmentVariable("TAVERNAGENT_REDUCE_MOTION");
        if (!string.IsNullOrEmpty(env))
            return env is "1" or "true" or "TRUE" or "on";

        if (!OperatingSystem.IsWindows()) return false;
        try
        {
            if (SystemParametersInfo(SpiGetClientAreaAnimation, 0, out var enabled, 0))
                return enabled == 0;
        }
        catch
        {
            // fall through to full motion
        }
        return false;
    }

    [DllImport("user32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool SystemParametersInfo(uint action, uint param, out int value, uint winIni);
}
