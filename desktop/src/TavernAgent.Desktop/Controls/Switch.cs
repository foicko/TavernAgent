using Avalonia;
using Avalonia.Controls;

namespace TavernAgent.Desktop.Controls;

/// <summary>Labeled toggle switch (web: ui/Switch). Label via Body.</summary>
public partial class Switch : UserControl
{
    public static readonly StyledProperty<bool> IsCheckedProperty =
        AvaloniaProperty.Register<Switch, bool>(nameof(IsChecked));

    public static readonly StyledProperty<object?> BodyProperty =
        AvaloniaProperty.Register<Switch, object?>(nameof(Body));

    public Switch()
    {
        InitializeComponent();
    }

    public bool IsChecked { get => GetValue(IsCheckedProperty); set => SetValue(IsCheckedProperty, value); }

    public object? Body { get => GetValue(BodyProperty); set => SetValue(BodyProperty, value); }
}
