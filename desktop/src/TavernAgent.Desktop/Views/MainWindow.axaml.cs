using System;
using Avalonia;
using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Interactivity;
using Avalonia.Media;
using Avalonia.VisualTree;
using TavernAgent.Desktop.Services;
using TavernAgent.Desktop.ViewModels;

namespace TavernAgent.Desktop.Views;

public partial class MainWindow : Window
{
    private double _previousWidth;

    public MainWindow()
    {
        InitializeComponent();
        Opened += OnOpened;

        // Flat opaque shell (beautifului): no native acrylic/mica blur.
        TransparencyLevelHint = new[] { WindowTransparencyLevel.None };
    }

    private ShellViewModel? Vm => DataContext as ShellViewModel;

    private void OnOpened(object? sender, EventArgs e)
    {
        _previousWidth = Bounds.Width;
        ApplyResponsiveRailBreakpoints(Bounds.Width, double.MaxValue);

        if (Vm is null) return;
        if (Vm.Phase == BootPhase.Failed)
        {
            // Already failed before window showed — keep overlay with error.
            return;
        }

        // Fast path: the sidecar may have become ready before the window opened,
        // in which case ContentReady already fired and the handler would never run.
        if (Vm.Workspace is not null)
        {
            ShowMain(true);
            return;
        }

        Vm.ContentReady += (_, _) => ShowMain(true);
    }

    protected override void OnPropertyChanged(AvaloniaPropertyChangedEventArgs change)
    {
        base.OnPropertyChanged(change);
        if (change.Property == BoundsProperty && change.NewValue is Rect newBounds)
        {
            var newWidth = newBounds.Width;
            if (newWidth > 0 && Math.Abs(newWidth - _previousWidth) > 1)
            {
                ApplyResponsiveRailBreakpoints(newWidth, _previousWidth);
                _previousWidth = newWidth;
            }
        }
    }

    private void ApplyResponsiveRailBreakpoints(double currentWidth, double prevWidth)
    {
        if (currentWidth <= 0) return;

        // When crossing below 1180px, collapse the right rail to give Stage breathing room.
        if (currentWidth < 1180 && prevWidth >= 1180)
        {
            AppServices.Ui.SetRightRailCollapsed(true);
        }

        // When crossing below 1000px, collapse the left rail to icon dock.
        if (currentWidth < 1000 && prevWidth >= 1000)
        {
            AppServices.Ui.SetLeftRailCollapsed(true);
        }
    }

    private void ShowMain(bool ready)
    {
        void Update()
        {
            var overlay = this.FindControl<Border>("BootOverlay");
            var main = this.FindControl<Control>("MainContent");
            if (overlay is not null) overlay.IsVisible = !ready;
            if (main is not null) main.IsVisible = ready;
        }

        if (Avalonia.Threading.Dispatcher.UIThread.CheckAccess()) Update();
        else Avalonia.Threading.Dispatcher.UIThread.Post(Update);
    }

    private void OnBootOverlayPointerPressed(object? sender, PointerPressedEventArgs e)
    {
        if (!e.GetCurrentPoint(this).Properties.IsLeftButtonPressed) return;
        if (e.Source is Visual visual && visual.FindAncestorOfType<Button>(includeSelf: true) is not null) return;

        if (e.ClickCount == 2)
        {
            WindowState = WindowState == WindowState.Maximized ? WindowState.Normal : WindowState.Maximized;
        }
        else
        {
            BeginMoveDrag(e);
        }
    }

    private void OnMinimizeClicked(object? sender, RoutedEventArgs e)
    {
        WindowState = WindowState.Minimized;
    }

    private void OnCloseClicked(object? sender, RoutedEventArgs e)
    {
        Close();
    }
}
