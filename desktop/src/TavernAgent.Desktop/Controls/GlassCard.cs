using Avalonia;
using Avalonia.Controls;

namespace TavernAgent.Desktop.Controls;

/// <summary>Translucent glass surface with specular edge (web: .glass / elevated modal).</summary>
public partial class GlassCard : UserControl
{
    public static readonly StyledProperty<object?> BodyProperty =
        AvaloniaProperty.Register<GlassCard, object?>(nameof(Body));

    public GlassCard()
    {
        InitializeComponent();
    }

    public object? Body { get => GetValue(BodyProperty); set => SetValue(BodyProperty, value); }
}
