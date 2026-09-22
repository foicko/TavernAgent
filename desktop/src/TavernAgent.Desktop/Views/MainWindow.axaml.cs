using Avalonia;
using Avalonia.Controls;
using Avalonia.Media;
using TavernAgent.Desktop.ViewModels;

namespace TavernAgent.Desktop.Views;

public partial class MainWindow : Window
{
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
