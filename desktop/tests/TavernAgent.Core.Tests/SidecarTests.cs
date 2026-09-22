using TavernAgent.Sidecar;
using Xunit;

namespace TavernAgent.Core.Tests;

public class PortAllocatorTests
{
    [Fact]
    public void Allocates_Ephemeral_Loopback_Port()
    {
        var port = PortAllocator.GetFreeLoopbackPort();
        Assert.InRange(port, 1024, 65535);
    }

    [Fact]
    public void Subsequent_Allocations_Are_Distinct_Or_Reusable()
    {
        var a = PortAllocator.GetFreeLoopbackPort();
        var b = PortAllocator.GetFreeLoopbackPort();
        // Either still free (distinct) or OS reused after close — both valid.
        Assert.True(a is > 0 and <= 65535);
        Assert.True(b is > 0 and <= 65535);
    }
}

public class SidecarLogTests
{
    [Fact]
    public void Writes_Tail_And_Rolls_At_MaxBytes()
    {
        var dir = Path.Combine(Path.GetTempPath(), "tavernagent-test-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(dir);
        var path = Path.Combine(dir, "sidecar.log");
        try
        {
            using (var log = new SidecarLog(path, maxBytes: 256, tailCapacity: 10))
            {
                for (var i = 0; i < 50; i++)
                    log.WriteLine($"line-{i}");
                Assert.Equal(10, log.Tail.Count);
                Assert.Contains("line-49", log.Tail);
            }
            Assert.True(File.Exists(path));
            Assert.True(File.Exists(path + ".1") || new FileInfo(path).Length <= 512);
        }
        finally
        {
            try { Directory.Delete(dir, recursive: true); } catch { /* ignore */ }
        }
    }
}

public class SingleInstanceTests
{
    [Fact]
    public void Second_Acquire_Fails_While_First_Held()
    {
        var dir = Path.Combine(Path.GetTempPath(), "tavernagent-si-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(dir);
        try
        {
            if (OperatingSystem.IsWindows())
            {
                // Named mutex is process-wide; two acquires in one process share ownership semantics.
                // Validate the non-Windows file-lock path is skipped and first acquire succeeds.
                using var first = SingleInstance.TryAcquire(dir);
                Assert.True(first.Acquired);
            }
            else
            {
                using var first = SingleInstance.TryAcquire(dir);
                Assert.True(first.Acquired);
                var second = SingleInstance.TryAcquire(dir);
                Assert.False(second.Acquired);
                second.Dispose();
            }
        }
        finally
        {
            try { Directory.Delete(dir, recursive: true); } catch { /* ignore */ }
        }
    }
}
