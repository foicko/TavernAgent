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
    private SingleInstance? _singleInstance;
    private ShellViewModel? _shell;

    public override void Initialize()
    {
        AvaloniaXamlLoader.Load(this);
        MotionPreference.Apply();
    }

    public override void OnFrameworkInitializationCompleted()
    {
        // Single instance must win BEFORE any sidecar start (data-dir lock order).
        _singleInstance = SingleInstance.TryAcquire(SidecarHost.DataDir);
        if (!_singleInstance.Acquired)
        {
            _singleInstance.ActivateExisting();
            if (ApplicationLifetime is IClassicDesktopStyleApplicationLifetime life)
            {
                life.Shutdown();
            }
            return;
        }

        if (ApplicationLifetime is IClassicDesktopStyleApplicationLifetime appLife)
        {
            _shell = new ShellViewModel();
            var window = new MainWindow
            {
                DataContext = _shell,
            };
            appLife.MainWindow = window;
            appLife.Exit += async (_, _) =>
            {
                try { await _shell.DisposeAsync(); }
                catch { /* process teardown */ }
                _singleInstance.Dispose();
            };
            _ = _shell.InitializeAsync();
        }

        base.OnFrameworkInitializationCompleted();
    }
}
