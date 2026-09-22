using Avalonia;
using Avalonia.Controls;
using Avalonia.Media;
using TavernAgent.Desktop.ViewModels;

namespace TavernAgent.Desktop.Views;

public partial class MainWindow : Window
{
    private bool _materialApplied;

    public MainWindow()
    {
        InitializeComponent();
        Opened += OnOpened;

        // Prefer native blur behind the window; falls back to an opaque base when unsupported.
        TransparencyLevelHint = new[]
        {
            WindowTransparencyLevel.AcrylicBlur,
            WindowTransparencyLevel.Mica,
            WindowTransparencyLevel.Blur,
            WindowTransparencyLevel.None,
        };
        PropertyChanged += OnWindowPropertyChanged;
    }

    private ShellViewModel? Vm => DataContext as ShellViewModel;

    private void OnWindowPropertyChanged(object? sender, AvaloniaPropertyChangedEventArgs e)
    {
        if (e.Property == ActualTransparencyLevelProperty) ApplyMaterial();
    }

    private void ApplyMaterial()
    {
        var active = ActualTransparencyLevel != WindowTransparencyLevel.None;
        if (active == _materialApplied) return;
        _materialApplied = active;
        if (Application.Current is not { } app) return;

        // Swap the ambient base so the native material shows through when available.
        app.Resources["AppBaseBrush"] = active
            ? app.FindResource("WindowBackgroundMaterialBrush")
            : app.FindResource("WindowBackgroundBrush");
    }

    private void OnOpened(object? sender, EventArgs e)
    {
        ApplyMaterial();
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
}
