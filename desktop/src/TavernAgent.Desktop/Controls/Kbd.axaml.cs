using Avalonia;
using Avalonia.Controls;

namespace TavernAgent.Desktop.Controls;

/// <summary>Small keyboard-key badge (web: Kbd).</summary>
public partial class Kbd : UserControl
{
    public static readonly StyledProperty<string?> TextProperty =
        AvaloniaProperty.Register<Kbd, string?>(nameof(Text));

    public Kbd()
    {
        InitializeComponent();
    }

    public string? Text { get => GetValue(TextProperty); set => SetValue(TextProperty, value); }
}
