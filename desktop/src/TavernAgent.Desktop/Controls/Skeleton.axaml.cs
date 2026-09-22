using Avalonia;
using Avalonia.Controls;

namespace TavernAgent.Desktop.Controls;

/// <summary>Pulsing placeholder block (web: .skeleton).</summary>
public partial class Skeleton : UserControl
{
    public static readonly StyledProperty<CornerRadius> RadiusProperty =
        AvaloniaProperty.Register<Skeleton, CornerRadius>(nameof(Radius), new CornerRadius(10));

    public Skeleton()
    {
        InitializeComponent();
    }

    public CornerRadius Radius { get => GetValue(RadiusProperty); set => SetValue(RadiusProperty, value); }
}
