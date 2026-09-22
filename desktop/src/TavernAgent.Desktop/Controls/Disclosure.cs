using Avalonia;
using Avalonia.Controls;

namespace TavernAgent.Desktop.Controls;

/// <summary>Collapsible disclosure (web: ui/Disclosure). Content via Body.</summary>
public partial class Disclosure : UserControl
{
    public static readonly StyledProperty<string?> HeaderProperty =
        AvaloniaProperty.Register<Disclosure, string?>(nameof(Header));

    public static readonly StyledProperty<bool> IsExpandedProperty =
        AvaloniaProperty.Register<Disclosure, bool>(nameof(IsExpanded));

    public static readonly StyledProperty<object?> BodyProperty =
        AvaloniaProperty.Register<Disclosure, object?>(nameof(Body));

    public Disclosure()
    {
        InitializeComponent();
    }

    public string? Header { get => GetValue(HeaderProperty); set => SetValue(HeaderProperty, value); }

    public bool IsExpanded { get => GetValue(IsExpandedProperty); set => SetValue(IsExpandedProperty, value); }

    public object? Body { get => GetValue(BodyProperty); set => SetValue(BodyProperty, value); }
}
