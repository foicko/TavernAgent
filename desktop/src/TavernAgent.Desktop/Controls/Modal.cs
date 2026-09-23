using System.Windows.Input;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Interactivity;
using Avalonia.Threading;

namespace TavernAgent.Desktop.Controls;

/// <summary>
/// Modal dialog primitive (web: ui/Modal). Host it in a top-level overlay Panel and
/// bind IsOpen. Scrim click / Esc / close button all collapse it. Body carries content.
/// </summary>
public partial class Modal : UserControl
{
    public static readonly StyledProperty<bool> IsOpenProperty =
        AvaloniaProperty.Register<Modal, bool>(nameof(IsOpen));

    public static readonly StyledProperty<string?> TitleProperty =
        AvaloniaProperty.Register<Modal, string?>(nameof(Title));

    public static readonly StyledProperty<double> DialogWidthProperty =
        AvaloniaProperty.Register<Modal, double>(nameof(DialogWidth), 720d);

    public static readonly StyledProperty<object?> BodyProperty =
        AvaloniaProperty.Register<Modal, object?>(nameof(Body));

    // When set, close gestures invoke this command instead of writing IsOpen directly.
    // IsOpen is usually one-way bound to a view model, so a local write would be lost.
    public static readonly StyledProperty<ICommand?> CloseCommandProperty =
        AvaloniaProperty.Register<Modal, ICommand?>(nameof(CloseCommand));

    public Modal()
    {
        InitializeComponent();
        IsVisible = IsOpen;
        Scrim.PointerPressed += (_, _) => RequestClose();
        CloseButton.Click += OnCloseClick;
        KeyDown += OnKeyDown;
        IsOpenProperty.Changed.AddClassHandler<Modal>((m, e) => m.OnIsOpenChanged((bool)e.NewValue!));
        DialogWidthProperty.Changed.AddClassHandler<Modal>((m, e) => m.UpdateCardConstraints(m.Bounds.Size));
    }

    public bool IsOpen { get => GetValue(IsOpenProperty); set => SetValue(IsOpenProperty, value); }

    public string? Title { get => GetValue(TitleProperty); set => SetValue(TitleProperty, value); }

    public double DialogWidth { get => GetValue(DialogWidthProperty); set => SetValue(DialogWidthProperty, value); }

    public object? Body { get => GetValue(BodyProperty); set => SetValue(BodyProperty, value); }

    public ICommand? CloseCommand { get => GetValue(CloseCommandProperty); set => SetValue(CloseCommandProperty, value); }

    private void OnCloseClick(object? sender, RoutedEventArgs e) => RequestClose();

    private void OnKeyDown(object? sender, KeyEventArgs e)
    {
        if (e.Key == Key.Escape) RequestClose();
    }

    private void RequestClose()
    {
        var command = CloseCommand;
        if (command is not null && command.CanExecute(null))
        {
            command.Execute(null);
            return;
        }

        IsOpen = false;
    }

    private void OnIsOpenChanged(bool open)
    {
        IsVisible = open;
        if (open)
        {
            UpdateCardConstraints(Bounds.Size);
            Dispatcher.UIThread.Post(() => Card.Focus(), DispatcherPriority.Input);
        }
    }

    protected override void OnSizeChanged(SizeChangedEventArgs e)
    {
        base.OnSizeChanged(e);
        UpdateCardConstraints(e.NewSize);
    }

    private void UpdateCardConstraints(Size size)
    {
        var targetWidth = DialogWidth > 0 ? DialogWidth : 720d;
        var clampedWidth = size.Width > 0
            ? Math.Max(280, Math.Min(targetWidth, size.Width - 32))
            : targetWidth;

        Card.Width = clampedWidth;
        Card.MaxWidth = clampedWidth;

        if (size.Height > 0)
        {
            Card.MaxHeight = Math.Max(200, size.Height - 32);
        }
    }
}
