using Avalonia;
using Avalonia.Controls;

namespace TavernAgent.Desktop.Controls;

/// <summary>Card section with optional indexed header (web: .ui-section). Content via Body.</summary>
public partial class Section : UserControl
{
    public static readonly StyledProperty<string?> TitleProperty =
        AvaloniaProperty.Register<Section, string?>(nameof(Title));

    public static readonly StyledProperty<string?> IndexProperty =
        AvaloniaProperty.Register<Section, string?>(nameof(Index));

    public static readonly StyledProperty<object?> BodyProperty =
        AvaloniaProperty.Register<Section, object?>(nameof(Body));

    public Section()
    {
        InitializeComponent();
    }

    public string? Title { get => GetValue(TitleProperty); set => SetValue(TitleProperty, value); }

    public string? Index { get => GetValue(IndexProperty); set => SetValue(IndexProperty, value); }

    public object? Body { get => GetValue(BodyProperty); set => SetValue(BodyProperty, value); }
}
