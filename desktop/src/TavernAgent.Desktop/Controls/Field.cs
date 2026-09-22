using Avalonia;
using Avalonia.Controls;

namespace TavernAgent.Desktop.Controls;

/// <summary>Labeled form field wrapper (web: .ui-field). Input via Body.</summary>
public partial class Field : UserControl
{
    public static readonly StyledProperty<string?> LabelProperty =
        AvaloniaProperty.Register<Field, string?>(nameof(Label));

    public static readonly StyledProperty<string?> HintProperty =
        AvaloniaProperty.Register<Field, string?>(nameof(Hint));

    public static readonly StyledProperty<string?> ErrorProperty =
        AvaloniaProperty.Register<Field, string?>(nameof(Error));

    public static readonly StyledProperty<object?> BodyProperty =
        AvaloniaProperty.Register<Field, object?>(nameof(Body));

    public Field()
    {
        InitializeComponent();
    }

    public string? Label { get => GetValue(LabelProperty); set => SetValue(LabelProperty, value); }

    public string? Hint { get => GetValue(HintProperty); set => SetValue(HintProperty, value); }

    public string? Error { get => GetValue(ErrorProperty); set => SetValue(ErrorProperty, value); }

    public object? Body { get => GetValue(BodyProperty); set => SetValue(BodyProperty, value); }
}
