using System.Text;

namespace TavernAgent.Sidecar;

public enum SidecarState
{
    Idle,
    Starting,
    Ready,
    Stopping,
    Stopped,
    Failed,
}

public sealed class SidecarStateChangedEventArgs : EventArgs
{
    public required SidecarState State { get; init; }
    public string? Error { get; init; }
}

public sealed class SidecarReadyEventArgs : EventArgs
{
    public required Uri BaseAddress { get; init; }
    public required int Port { get; init; }
}

/// <summary>
/// Launches the headless Go server as a child process, waits for /healthz,
/// drains logs continuously, and guarantees teardown (CTRL_BREAK/SIGTERM → grace → kill;
/// Job Object as crash fallback on Windows).
/// </summary>
public sealed class SidecarSupervisor : IAsyncDisposable
{
    private readonly HttpClient _probe = new() { Timeout = TimeSpan.FromSeconds(1) };
    private readonly object _gate = new();
    private SidecarChild? _child;
    private SidecarLog? _log;
    private CancellationTokenSource? _lifetime;
    private Task? _waitTask;
    private SidecarState _state = SidecarState.Idle;
    private string? _lastError;
    private int _port;
    private bool _stopping;

    public SidecarSupervisor(SidecarOptions options)
    {
        Options = options;
    }

    public SidecarOptions Options { get; }

    public SidecarState State
    {
        get { lock (_gate) return _state; }
    }

    public string? LastError
    {
        get { lock (_gate) return _lastError; }
    }

    public int Port
    {
        get { lock (_gate) return _port; }
    }

    public Uri? BaseAddress { get; private set; }

    public SidecarLog? Log
    {
        get { lock (_gate) return _log; }
    }

    public event EventHandler<SidecarStateChangedEventArgs>? StateChanged;

    public event EventHandler<SidecarReadyEventArgs>? Ready;

    public async Task StartAsync(CancellationToken cancellationToken = default)
    {
        lock (_gate)
        {
            if (_state != SidecarState.Idle)
                throw new InvalidOperationException($"Sidecar already started (state={_state}).");
            SetStateLocked(SidecarState.Starting, null);
        }
        Notify();

        if (!File.Exists(Options.ExecutablePath))
        {
            Fail($"找不到无头服务可执行文件：{Options.ExecutablePath}{Environment.NewLine}" +
                 $"（请先构建 Go 产物，或将 tavernagent 放到应用同目录。）");
            return;
        }

        var dataDir = Path.GetFullPath(Options.DataDir);
        Directory.CreateDirectory(dataDir);

        var logDir = string.IsNullOrEmpty(Options.LogDir) ? Path.Combine(dataDir, "logs") : Options.LogDir;
        var logPath = Path.Combine(logDir, "sidecar.log");
        var log = new SidecarLog(logPath, Options.LogMaxBytes);
        lock (_gate) _log = log;

        var port = Options.FixedAddr is not null
            ? ParsePort(Options.FixedAddr)
            : PortAllocator.GetFreeLoopbackPort();
        var addr = Options.FixedAddr ?? $"127.0.0.1:{port}";
        lock (_gate)
        {
            _port = port;
            _lifetime = new CancellationTokenSource();
        }

        SidecarChild child;
        try
        {
            child = SidecarChild.Spawn(Options, dataDir, addr);
        }
        catch (Exception ex)
        {
            Fail($"启动无头服务失败：{ex.Message}");
            return;
        }

        lock (_gate) _child = child;

        log.WriteLine($"[sidecar] spawned pid={child.Id} addr={addr} data={dataDir}");
        var ct = _lifetime!.Token;
        _ = Task.Run(() => PumpAsync(child.StdoutReader, log, "out", ct), CancellationToken.None);
        _ = Task.Run(() => PumpAsync(child.StderrReader, log, "err", ct), CancellationToken.None);
        _waitTask = Task.Run(() => WatchExitAsync(child, log), CancellationToken.None);

        var baseAddr = new Uri($"http://127.0.0.1:{port}/");
        var ready = await WaitForHealthAsync(baseAddr, child, cancellationToken).ConfigureAwait(false);
        if (!ready)
        {
            await Task.Delay(120, CancellationToken.None).ConfigureAwait(false);
            lock (_gate)
            {
                if (_state == SidecarState.Starting)
                {
                    var tail = string.Join(Environment.NewLine, log.Tail.TakeLast(30));
                    FailStateLocked(
                        $"无头服务在 {Options.ReadyTimeout.TotalSeconds:0}s 内未就绪。" +
                        (string.IsNullOrWhiteSpace(tail) ? "" : Environment.NewLine + tail));
                }
            }
            Notify();
            return;
        }

        BaseAddress = baseAddr;
        lock (_gate) SetStateLocked(SidecarState.Ready, null);
        Notify();
        Ready?.Invoke(this, new SidecarReadyEventArgs { BaseAddress = baseAddr, Port = port });
    }

    public async Task StopAsync()
    {
        SidecarChild? child;
        CancellationTokenSource? lifetime;
        SidecarLog? log;
        lock (_gate)
        {
            if (_stopping)
            {
                child = _child;
                lifetime = _lifetime;
                log = _log;
            }
            else if (_child is null && _state is SidecarState.Idle or SidecarState.Stopped or SidecarState.Failed)
            {
                return;
            }
            else
            {
                _stopping = true;
                SetStateLocked(SidecarState.Stopping, _lastError);
                child = _child;
                lifetime = _lifetime;
                log = _log;
            }
        }
        Notify();

        try { lifetime?.Cancel(); } catch { /* ignore */ }

        if (child is not null && !child.HasExited)
        {
            log?.WriteLine("[sidecar] sending graceful stop");
            var signalled = child.TrySendGracefulStop();
            log?.WriteLine(signalled ? "[sidecar] signal delivered" : "[sidecar] signal not delivered");
            await child.WaitForExitAsync(Options.ShutdownGrace).ConfigureAwait(false);
            if (!child.HasExited)
            {
                log?.WriteLine("[sidecar] grace period elapsed; killing process");
                child.Kill();
                await child.WaitForExitAsync(TimeSpan.FromSeconds(2)).ConfigureAwait(false);
            }
        }

        if (_waitTask is not null)
        {
            try { await _waitTask.WaitAsync(TimeSpan.FromSeconds(2)).ConfigureAwait(false); }
            catch { /* ignore */ }
        }

        lock (_gate)
        {
            if (_state != SidecarState.Failed)
                SetStateLocked(SidecarState.Stopped, null);
            _stopping = false;
        }
        Notify();
    }

    public async ValueTask DisposeAsync()
    {
        await StopAsync().ConfigureAwait(false);
        _probe.Dispose();
        lock (_gate)
        {
            _child?.Dispose();
            _child = null;
            _lifetime?.Dispose();
            _lifetime = null;
            _log?.Dispose();
            _log = null;
        }
    }

    private async Task<bool> WaitForHealthAsync(Uri baseAddr, SidecarChild child, CancellationToken ct)
    {
        var deadline = DateTime.UtcNow + Options.ReadyTimeout;
        using var linked = CancellationTokenSource.CreateLinkedTokenSource(ct);
        while (DateTime.UtcNow < deadline && !linked.IsCancellationRequested)
        {
            if (child.HasExited) return false;
            try
            {
                using var timeout = CancellationTokenSource.CreateLinkedTokenSource(linked.Token);
                timeout.CancelAfter(TimeSpan.FromSeconds(1));
                var resp = await _probe.GetAsync(new Uri(baseAddr, "healthz"), timeout.Token).ConfigureAwait(false);
                if (resp.IsSuccessStatusCode) return true;
            }
            catch (OperationCanceledException) when (linked.IsCancellationRequested)
            {
                return false;
            }
            catch
            {
                // connection refused while starting
            }

            try { await Task.Delay(150, linked.Token).ConfigureAwait(false); }
            catch (OperationCanceledException) { return false; }
        }
        return false;
    }

    private async Task WatchExitAsync(SidecarChild child, SidecarLog log)
    {
        await child.WaitForExitAsync(TimeSpan.FromMinutes(30)).ConfigureAwait(false);
        // Brief delay so pending log pump lines land first.
        try { await Task.Delay(80).ConfigureAwait(false); } catch { /* ignore */ }
        log.WriteLine($"[sidecar] exited code={child.ExitCode}");

        lock (_gate)
        {
            if (_stopping)
            {
                // StopAsync owns the final state.
            }
            else if (_state is SidecarState.Starting or SidecarState.Ready)
            {
                var tail = string.Join(Environment.NewLine, log.Tail.TakeLast(30));
                FailStateLocked(
                    $"无头服务意外退出（code={child.ExitCode}）。" +
                    (string.IsNullOrWhiteSpace(tail) ? "" : Environment.NewLine + tail));
            }
        }
        Notify();
    }

    private static async Task PumpAsync(StreamReader reader, SidecarLog log, string tag, CancellationToken ct)
    {
        try
        {
            while (!ct.IsCancellationRequested)
            {
                var line = await reader.ReadLineAsync(ct).ConfigureAwait(false);
                if (line is null) break;
                log.WriteLine($"[{tag}] {line}");
            }
        }
        catch
        {
            // reader closed on process exit
        }
    }

    private void Fail(string message)
    {
        lock (_gate)
        {
            FailStateLocked(message);
            _log?.WriteLine($"[sidecar] failed: {message}");
        }
        Notify();
    }

    private void FailStateLocked(string message)
    {
        _lastError = message;
        _state = SidecarState.Failed;
    }

    private void SetStateLocked(SidecarState state, string? error)
    {
        _state = state;
        if (error is not null) _lastError = error;
        if (state is SidecarState.Ready or SidecarState.Idle) _lastError = null;
    }

    private void Notify()
    {
        SidecarState state;
        string? error;
        lock (_gate)
        {
            state = _state;
            error = _lastError;
        }
        StateChanged?.Invoke(this, new SidecarStateChangedEventArgs { State = state, Error = error });
    }

    private static int ParsePort(string addr)
    {
        var idx = addr.LastIndexOf(':');
        if (idx < 0 || !int.TryParse(addr.AsSpan(idx + 1), out var port))
            throw new ArgumentException($"Invalid addr: {addr}", nameof(addr));
        return port;
    }
}
