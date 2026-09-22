using Avalonia;
using Avalonia.Controls;

namespace TavernAgent.Desktop.Controls;

/// <summary>Centered empty placeholder (web: .ui-empty). Actions via Body.</summary>
public partial class EmptyState : UserControl
{
    public static readonly StyledProperty<string?> IconProperty =
        AvaloniaProperty.Register<EmptyState, string?>(nameof(Icon));

    public static readonly StyledProperty<string?> TitleProperty =
        AvaloniaProperty.Register<EmptyState, string?>(nameof(Title));

    public static readonly StyledProperty<string?> DescriptionProperty =
        AvaloniaProperty.Register<EmptyState, string?>(nameof(Description));

    public static readonly StyledProperty<object?> BodyProperty =
        AvaloniaProperty.Register<EmptyState, object?>(nameof(Body));

    public EmptyState()
    {
        InitializeComponent();
    }

    public string? Icon { get => GetValue(IconProperty); set => SetValue(IconProperty, value); }

    public string? Title { get => GetValue(TitleProperty); set => SetValue(TitleProperty, value); }

    public string? Description { get => GetValue(DescriptionProperty); set => SetValue(DescriptionProperty, value); }

    public object? Body { get => GetValue(BodyProperty); set => SetValue(BodyProperty, value); }
}
