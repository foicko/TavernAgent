namespace TavernAgent.Sidecar;

/// <summary>Options for launching and supervising the headless Go sidecar.</summary>
public sealed class SidecarOptions
{
    /// <summary>Path to the sidecar executable (tavernagent / tavernagent.exe).</summary>
    public required string ExecutablePath { get; init; }

    /// <summary>Data directory passed as -data. Absolute path required.</summary>
    public required string DataDir { get; init; }

    /// <summary>
    /// Optional fixed loopback address (e.g. "127.0.0.1:8890").
    /// When null a free ephemeral port is allocated by the parent first.
    /// </summary>
    public string? FixedAddr { get; init; }

    /// <summary>Provider fallback for -provider (only "mock" or null).</summary>
    public string? Provider { get; init; }

    /// <summary>Extra args appended after the standard flags.</summary>
    public IReadOnlyList<string> ExtraArgs { get; init; } = Array.Empty<string>();

    /// <summary>Maximum time to wait for GET /healthz after spawn.</summary>
    public TimeSpan ReadyTimeout { get; init; } = TimeSpan.FromSeconds(15);

    /// <summary>Grace period for CTRL_BREAK shutdown before hard kill.</summary>
    public TimeSpan ShutdownGrace { get; init; } = TimeSpan.FromSeconds(5);

    /// <summary>Directory for sidecar.log. Defaults to &lt;DataDir&gt;/logs.</summary>
    public string? LogDir { get; init; }

    /// <summary>Max sidecar.log size before rolling to .1 (bytes). Default 2 MiB.</summary>
    public long LogMaxBytes { get; init; } = 2 * 1024 * 1024;
}
