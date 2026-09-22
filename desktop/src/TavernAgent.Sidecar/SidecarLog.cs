using System.Text;

namespace TavernAgent.Sidecar;

/// <summary>
/// Rolling file sink for sidecar stdout/stderr with an in-memory tail
/// used by the startup error page.
/// </summary>
public sealed class SidecarLog : IDisposable
{
    private readonly object _gate = new();
    private readonly Queue<string> _tail = new();
    private readonly int _tailCapacity;
    private StreamWriter? _writer;
    private long _written;
    private bool _disposed;

    public SidecarLog(string path, long maxBytes = 2 * 1024 * 1024, int tailCapacity = 200)
    {
        Path = path;
        MaxBytes = maxBytes;
        _tailCapacity = tailCapacity;
        var dir = System.IO.Path.GetDirectoryName(path);
        if (!string.IsNullOrEmpty(dir)) Directory.CreateDirectory(dir);

        // Rotate only when the existing file already exceeds the budget.
        if (File.Exists(path))
        {
            var len = new FileInfo(path).Length;
            if (len >= maxBytes)
            {
                var backup = path + ".1";
                if (File.Exists(backup)) File.Delete(backup);
                File.Move(path, backup);
                _written = 0;
            }
            else
            {
                _written = len;
            }
        }

        _writer = new StreamWriter(
            new FileStream(path, FileMode.Append, FileAccess.Write, FileShare.Read),
            Encoding.UTF8)
        {
            AutoFlush = true,
        };
    }

    public string Path { get; }

    public long MaxBytes { get; }

    /// <summary>Recent log lines (oldest first) for the error UI.</summary>
    public IReadOnlyList<string> Tail
    {
        get
        {
            lock (_gate) return _tail.ToArray();
        }
    }

    public void WriteLine(string line)
    {
        if (_disposed) return;
        lock (_gate)
        {
            if (_disposed) return;
            _tail.Enqueue(line);
            while (_tail.Count > _tailCapacity) _tail.Dequeue();
            if (_writer is null) return;
            try
            {
                _writer.WriteLine(line);
                _written += Encoding.UTF8.GetByteCount(line) + Environment.NewLine.Length;
                if (_written >= MaxBytes) RollLocked();
            }
            catch (IOException)
            {
                // Never let logging take down process supervision.
            }
        }
    }

    public void Dispose()
    {
        lock (_gate)
        {
            if (_disposed) return;
            _disposed = true;
            _writer?.Dispose();
            _writer = null;
        }
    }

    private void RollLocked()
    {
        _writer?.Dispose();
        _writer = null;
        var backup = Path + ".1";
        if (File.Exists(backup)) File.Delete(backup);
        if (File.Exists(Path)) File.Move(Path, backup);
        _written = 0;
        _writer = new StreamWriter(
            new FileStream(Path, FileMode.Create, FileAccess.Write, FileShare.Read),
            Encoding.UTF8)
        {
            AutoFlush = true,
        };
    }
}
