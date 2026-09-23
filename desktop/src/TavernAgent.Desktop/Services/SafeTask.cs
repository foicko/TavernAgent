using System.Diagnostics;

namespace TavernAgent.Desktop.Services;

/// <summary>
/// Fire-and-forget helpers that never let an unobserved exception reach the
/// dispatcher/AppDomain (which would terminate the process with no UI).
/// Use <c>SomeAsync().SafeFireAndForget()</c> instead of <c>_ = SomeAsync()</c>.
/// </summary>
public static class SafeTask
{
    public static void SafeFireAndForget(this Task task, Action<Exception>? onError = null)
    {
        if (task is null) return;
        _ = ObserveAsync(task, onError);
    }

    private static async Task ObserveAsync(Task task, Action<Exception>? onError)
    {
        try
        {
            await task.ConfigureAwait(false);
        }
        catch (OperationCanceledException)
        {
            // Expected during navigation / teardown.
        }
        catch (Exception ex)
        {
            if (onError is not null)
            {
                try { onError(ex); }
                catch (Exception handlerEx) { CrashLog.Write(handlerEx, "SafeTask.handler"); }
            }
            else
            {
                Debug.WriteLine($"[SafeTask] unhandled background failure: {ex}");
                CrashLog.Write(ex, "SafeTask");
            }
        }
    }
}
