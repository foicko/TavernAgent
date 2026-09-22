using Avalonia.Threading;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.Services;
using TavernAgent.Sidecar;

namespace TavernAgent.Desktop.ViewModels;

public enum BootPhase
{
    StartingSidecar,
    Ready,
    Failed,
}

/// <summary>
/// Top-level shell: drives sidecar lifecycle, exposes boot UI state,
/// and switches between splash / workspace.
/// </summary>
public sealed partial class ShellViewModel : ViewModelBase, IAsyncDisposable
{
    private SidecarSupervisor _supervisor;
    private readonly DesktopSettings _settings;
    private CancellationTokenSource? _bootCts;
    private bool _disposed;

    public ShellViewModel()
    {
        _settings = DesktopSettings.Load();
        _supervisor = new SidecarSupervisor(SidecarHost.CreateOptions());
        _supervisor.StateChanged += OnSidecarStateChanged;
        _phase = BootPhase.StartingSidecar;
        _statusText = "正在启动本地服务…";
        _theme = ResolveTheme(_settings.Theme);
        AppServices.Ui.SetTheme(_theme == "light" ? ThemeMode.Light : ThemeMode.Dark);
        AppServices.Ui.PropertyChanged += OnUiChanged;
    }

    [ObservableProperty]
    private BootPhase _phase;

    [ObservableProperty]
    private string _statusText = "";

    [ObservableProperty]
    private string? _errorText;

    [ObservableProperty]
    private string _theme = "dark";

    [ObservableProperty]
    private int _sidecarPort;

    [ObservableProperty]
    private string _logTail = "";

    [ObservableProperty]
    private WorkspaceViewModel? _workspace;

    public Uri? BaseAddress => _supervisor.BaseAddress;

    public SidecarState SidecarState => _supervisor.State;

    public event EventHandler? ContentReady;

    [RelayCommand]
    public async Task InitializeAsync()
    {
        ApplyTheme();
        _bootCts?.Cancel();
        _bootCts = new CancellationTokenSource();
        try
        {
            await _supervisor.StartAsync(_bootCts.Token).ConfigureAwait(true);
        }
        catch (Exception ex)
        {
            Phase = BootPhase.Failed;
            ErrorText = ex.Message;
            StatusText = "启动失败";
            RefreshDiagnostics();
            return;
        }

        if (_supervisor.State == SidecarState.Ready && _supervisor.BaseAddress is not null)
        {
            Phase = BootPhase.Ready;
            StatusText = "就绪";
            SidecarPort = _supervisor.Port;
            AppServices.Connect(_supervisor.BaseAddress);
            AppServices.Story.StateChanged += OnStoryChanged;

            try
            {
                Workspace = new WorkspaceViewModel();
            }
            catch (Exception ex)
            {
                // A broken view/VM must surface on the splash, not yield a blank window.
                Phase = BootPhase.Failed;
                ErrorText = "工作台初始化失败：" + ex;
                StatusText = "启动失败";
                RefreshDiagnostics();
                return;
            }

            ContentReady?.Invoke(this, EventArgs.Empty);

            try
            {
                await AppServices.LoadInitialAsync(_bootCts.Token).ConfigureAwait(true);
                Workspace.NotifyStoryChanged();
            }
            catch (Exception ex)
            {
                // Non-fatal: the workspace still renders and can retry via the UI.
                ErrorText = ex.Message;
            }
        }
        else
        {
            Phase = BootPhase.Failed;
            ErrorText = _supervisor.LastError;
            StatusText = "启动失败";
            RefreshDiagnostics();
        }
    }

    [RelayCommand]
    private async Task RetryAsync()
    {
        if (Phase != BootPhase.Failed) return;
        Phase = BootPhase.StartingSidecar;
        ErrorText = null;
        StatusText = "正在重试…";
        LogTail = "";
        try { await _supervisor.DisposeAsync().ConfigureAwait(true); }
        catch { /* ignore */ }

        _supervisor = new SidecarSupervisor(SidecarHost.CreateOptions());
        _supervisor.StateChanged += OnSidecarStateChanged;
        await InitializeAsync().ConfigureAwait(true);
    }

    [RelayCommand]
    private void ToggleTheme() => AppServices.Ui.ToggleTheme();

    partial void OnThemeChanged(string value)
    {
        _settings.Theme = value;
        _settings.Save();
    }

    public async ValueTask DisposeAsync()
    {
        if (_disposed) return;
        _disposed = true;
        try { _bootCts?.Cancel(); } catch { /* ignore */ }
        try { await _supervisor.DisposeAsync().ConfigureAwait(false); }
        catch { /* ignore */ }
        AppServices.Story.StateChanged -= OnStoryChanged;
        AppServices.Ui.PropertyChanged -= OnUiChanged;
        Workspace?.Dispose();
        _bootCts?.Dispose();
    }

    public override void Dispose()
    {
        DisposeAsync().AsTask().GetAwaiter().GetResult();
        GC.SuppressFinalize(this);
    }

    /// <summary>User choice wins; otherwise follow the OS light/dark preference.</summary>
    private static string ResolveTheme(string saved)
    {
        if (saved is "light" or "dark") return saved;
        var variant = Avalonia.Application.Current?.PlatformSettings?.GetColorValues().ThemeVariant;
        return variant == Avalonia.Platform.PlatformThemeVariant.Light ? "light" : "dark";
    }

    private void ApplyTheme()
    {
        var mode = Theme == "light" ? Avalonia.Styling.ThemeVariant.Light : Avalonia.Styling.ThemeVariant.Dark;
        Dispatcher.UIThread.Post(() =>
        {
            if (Avalonia.Application.Current is { } app)
                app.RequestedThemeVariant = mode;
        });
    }

    private void OnUiChanged(object? sender, System.ComponentModel.PropertyChangedEventArgs e)
    {
        if (e.PropertyName != nameof(UiStore.Theme)) return;
        Theme = AppServices.Ui.Theme == ThemeMode.Light ? "light" : "dark";
        ApplyTheme();
    }

    private void OnStoryChanged(object? sender, EventArgs e) => Workspace?.NotifyStoryChanged();

    private void RefreshDiagnostics()
    {
        LogTail = _supervisor.Log is null
            ? ""
            : string.Join(Environment.NewLine, _supervisor.Log.Tail.TakeLast(40));
    }

    private void OnSidecarStateChanged(object? sender, SidecarStateChangedEventArgs e)
    {
        void Apply()
        {
            if (e.State == SidecarState.Failed)
            {
                var wasBooting = Phase == BootPhase.StartingSidecar;
                var wasReady = Phase == BootPhase.Ready;
                if (wasBooting || wasReady)
                {
                    Phase = BootPhase.Failed;
                    ErrorText = e.Error;
                    StatusText = wasReady ? "服务已停止" : "启动失败";
                    RefreshDiagnostics();
                }
            }
            OnPropertyChanged(nameof(SidecarState));
        }

        if (Dispatcher.UIThread.CheckAccess()) Apply();
        else Dispatcher.UIThread.Post(Apply);
    }
}
