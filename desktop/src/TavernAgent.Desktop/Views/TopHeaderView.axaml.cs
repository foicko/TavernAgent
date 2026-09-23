using System;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Interactivity;
using Avalonia.VisualTree;
using AvaloniaPath = Avalonia.Controls.Shapes.Path;

namespace TavernAgent.Desktop.Views;

public partial class TopHeaderView : UserControl
{
    private Window? _subscribedWindow;

    public TopHeaderView()
    {
        InitializeComponent();
        ApplyPlatformChromeInsets();
    }

    private void ApplyPlatformChromeInsets()
    {
        var border = this.FindControl<Border>("HeaderBorder");
        var captionStack = this.FindControl<StackPanel>("CaptionButtonsStack");
        var captionDivider = this.FindControl<Border>("CaptionDivider");

        if (OperatingSystem.IsMacOS())
        {
            // macOS: native traffic lights reside at the top-left corner (~76 DIPs).
            // Reserve left padding so brand/breadcrumb doesn't overlap traffic lights.
            if (border is not null) border.Padding = new Thickness(78, 0, 14, 0);
            if (captionStack is not null) captionStack.IsVisible = false;
            if (captionDivider is not null) captionDivider.IsVisible = false;
        }
        else
        {
            // Windows & Linux: custom caption buttons integrated on the right side.
            if (border is not null) border.Padding = new Thickness(14, 0, 8, 0);
            if (captionStack is not null) captionStack.IsVisible = true;
            if (captionDivider is not null) captionDivider.IsVisible = true;
        }
    }

    protected override void OnAttachedToVisualTree(VisualTreeAttachmentEventArgs e)
    {
        base.OnAttachedToVisualTree(e);
        if (VisualRoot is Window window)
        {
            _subscribedWindow = window;
            window.PropertyChanged += OnWindowPropertyChanged;
            UpdateWindowState(window.WindowState);
        }
    }

    protected override void OnDetachedFromVisualTree(VisualTreeAttachmentEventArgs e)
    {
        if (_subscribedWindow is not null)
        {
            _subscribedWindow.PropertyChanged -= OnWindowPropertyChanged;
            _subscribedWindow = null;
        }
        base.OnDetachedFromVisualTree(e);
    }

    private void OnWindowPropertyChanged(object? sender, AvaloniaPropertyChangedEventArgs e)
    {
        if (e.Property == Window.WindowStateProperty && e.NewValue is WindowState state)
        {
            UpdateWindowState(state);
        }
    }

    private void UpdateWindowState(WindowState state)
    {
        var isMaximized = state == WindowState.Maximized;
        var iconMax = this.FindControl<AvaloniaPath>("IconMaximize");
        var iconRestore = this.FindControl<AvaloniaPath>("IconRestore");
        var btnMax = this.FindControl<Button>("BtnMaximize");

        if (iconMax is not null) iconMax.IsVisible = !isMaximized;
        if (iconRestore is not null) iconRestore.IsVisible = isMaximized;
        if (btnMax is not null) ToolTip.SetTip(btnMax, isMaximized ? "向下还原" : "最大化");
    }

    private void OnMinimizeClicked(object? sender, RoutedEventArgs e)
    {
        var window = VisualRoot as Window ?? TopLevel.GetTopLevel(this) as Window;
        if (window is not null)
        {
            window.WindowState = WindowState.Minimized;
        }
    }

    private void OnMaximizeClicked(object? sender, RoutedEventArgs e)
    {
        var window = VisualRoot as Window ?? TopLevel.GetTopLevel(this) as Window;
        if (window is not null)
        {
            window.WindowState = window.WindowState == WindowState.Maximized
                ? WindowState.Normal
                : WindowState.Maximized;
        }
    }

    private void OnCloseClicked(object? sender, RoutedEventArgs e)
    {
        var window = VisualRoot as Window ?? TopLevel.GetTopLevel(this) as Window;
        window?.Close();
    }

    private void OnHeaderPointerPressed(object? sender, PointerPressedEventArgs e)
    {
        if (!e.GetCurrentPoint(this).Properties.IsLeftButtonPressed) return;

        // Skip interactive elements (buttons, inputs)
        if (e.Source is Visual visual && visual.FindAncestorOfType<Button>(includeSelf: true) is not null)
        {
            return;
        }

        var window = VisualRoot as Window ?? TopLevel.GetTopLevel(this) as Window;
        if (window is null) return;

        if (e.ClickCount == 2)
        {
            window.WindowState = window.WindowState == WindowState.Maximized
                ? WindowState.Normal
                : WindowState.Maximized;
        }
        else
        {
            window.BeginMoveDrag(e);
        }
    }
}
