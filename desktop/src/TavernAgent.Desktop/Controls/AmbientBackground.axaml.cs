using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.Shapes;
using TavernAgent.Desktop.Services;

namespace TavernAgent.Desktop.Controls;

/// <summary>
/// Slow-drifting ambient gradient orbs behind the workspace. Purely decorative.
/// Intensity is bound by the shell (raised in immersive / reading mode).
/// The orb drift is declared in this control's own styles, so reduced-motion has
/// to be honoured here (a later app-level style cannot outrank a local one).
/// </summary>
public partial class AmbientBackground : UserControl
{
    public static readonly StyledProperty<double> IntensityProperty =
        AvaloniaProperty.Register<AmbientBackground, double>(nameof(Intensity), 0.85);

    public AmbientBackground()
    {
        InitializeComponent();
        if (!MotionPreference.ReduceMotion) StartDrift();
    }

    public double Intensity { get => GetValue(IntensityProperty); set => SetValue(IntensityProperty, value); }

    /// <summary>
    /// The orb drift lives in a <c>.drift</c>-gated style, so simply not applying the
    /// class keeps the orbs static when the user asked for reduced motion.
    /// </summary>
    private void StartDrift()
    {
        foreach (var orb in new[] { "OrbA", "OrbB", "OrbC" })
        {
            if (this.FindControl<Ellipse>(orb) is { } ellipse)
            {
                ellipse.Classes.Add("drift");
            }
        }
    }
}
