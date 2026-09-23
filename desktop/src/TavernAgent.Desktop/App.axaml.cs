using Avalonia;
using Avalonia.Controls.ApplicationLifetimes;
using Avalonia.Markup.Xaml;
using TavernAgent.Desktop.Services;
using TavernAgent.Desktop.ViewModels;
using TavernAgent.Desktop.Views;
using TavernAgent.Sidecar;

namespace TavernAgent.Desktop;

public class App : Application
{
    private ShellViewModel? _shell;

    public override void Initialize()
    {
        AvaloniaXamlLoader.Load(this);
        MotionPreference.Apply();
    }

    public override void OnFrameworkInitializationCompleted()
    {
        if (ApplicationLifetime is IClassicDesktopStyleApplicationLifetime appLife)
        {
            _shell = new ShellViewModel();
            var window = new MainWindow
            {
                DataContext = _shell,
            };
            appLife.MainWindow = window;
            window.Show();
            window.Activate();
            appLife.Exit += async (_, _) =>
            {
                try { await _shell.DisposeAsync(); }
                catch { /* process teardown */ }
            };
            _ = _shell.InitializeAsync();
        }

        base.OnFrameworkInitializationCompleted();
    }
}
