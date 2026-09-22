using Avalonia;
using Avalonia.Controls;

namespace TavernAgent.Desktop.Controls;

/// <summary>Inline notice (web: ui/Banner). Tones via Classes; body via Body.</summary>
public partial class Banner : UserControl
{
    public static readonly StyledProperty<string?> TitleProperty =
        AvaloniaProperty.Register<Banner, string?>(nameof(Title));

    public static readonly StyledProperty<string?> MessageProperty =
        AvaloniaProperty.Register<Banner, string?>(nameof(Message));

    public static readonly StyledProperty<object?> BodyProperty =
        AvaloniaProperty.Register<Banner, object?>(nameof(Body));

    public Banner()
    {
        InitializeComponent();
    }

    public string? Title { get => GetValue(TitleProperty); set => SetValue(TitleProperty, value); }

    public string? Message { get => GetValue(MessageProperty); set => SetValue(MessageProperty, value); }

    public object? Body { get => GetValue(BodyProperty); set => SetValue(BodyProperty, value); }
}
