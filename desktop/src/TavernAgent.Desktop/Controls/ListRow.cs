using Avalonia;
using Avalonia.Controls;

namespace TavernAgent.Desktop.Controls;

/// <summary>Generic list row (web: ui/ListRow). Add Class "interactive"; content via Body.</summary>
public partial class ListRow : UserControl
{
    public static readonly StyledProperty<object?> BodyProperty =
        AvaloniaProperty.Register<ListRow, object?>(nameof(Body));

    public ListRow()
    {
        InitializeComponent();
    }

    public object? Body { get => GetValue(BodyProperty); set => SetValue(BodyProperty, value); }
}
