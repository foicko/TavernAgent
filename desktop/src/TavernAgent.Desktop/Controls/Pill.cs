using Avalonia;
using Avalonia.Controls;

namespace TavernAgent.Desktop.Controls;

/// <summary>Compact glass chip (web: .ui-pill). Tones via Classes; content via Body.</summary>
public partial class Pill : UserControl
{
    public static readonly StyledProperty<object?> BodyProperty =
        AvaloniaProperty.Register<Pill, object?>(nameof(Body));

    public Pill()
    {
        InitializeComponent();
    }

    public object? Body { get => GetValue(BodyProperty); set => SetValue(BodyProperty, value); }
}
