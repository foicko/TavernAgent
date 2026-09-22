using System.Text;
using System.Text.Json;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Core.Api;

public sealed class SseHandlers
{
    public string? LastEventId { get; set; }
    public required Action<SSEEvent> OnEvent { get; set; }
    public required Action<Exception> OnFinalError { get; set; }
    public Action<int>? OnReconnect { get; set; }
}

/// <summary>
/// SSE client with Last-Event-ID resume, aggregateId:seq dedupe, stall watchdog,
/// and bounded exponential backoff — parity with web/src/app/api.ts subscribeEvents.
/// </summary>
public sealed class SseClient
{
    public static readonly TimeSpan[] DefaultBackoff =
    {
        TimeSpan.FromSeconds(1), TimeSpan.FromSeconds(2), TimeSpan.FromSeconds(4),
        TimeSpan.FromSeconds(8), TimeSpan.FromSeconds(16),
    };

    public static readonly TimeSpan StallTimeout = TimeSpan.FromSeconds(45);

    private static readonly HashSet<string> TerminalEvents = new(StringComparer.Ordinal)
    {
        "turn.committed", "turn.cancelled", "turn.failed", "turn.conflicted",
        "director.completed", "director.failed", "director.cancelled", "director.interrupted",
    };

    private readonly HttpClient _http;

    public SseClient(HttpClient http) => _http = http;

    public static SseEventBlock? ParseSseBlock(string raw, string prevId)
    {
        var eventName = "message";
        var id = prevId;
        var dataLines = new List<string>();
        var lines = raw.Split('\n');
        foreach (var line0 in lines)
        {
            var line = line0.EndsWith('\r') ? line0[..^1] : line0;
            if (line.StartsWith(':')) continue; // comment (heartbeat ping)
            if (line.StartsWith("event:", StringComparison.Ordinal))
                eventName = line[6..].Trim();
            else if (line.StartsWith("id:", StringComparison.Ordinal))
                id = line[3..].Trim();
            else if (line.StartsWith("data:", StringComparison.Ordinal))
            {
                var v = line[5..];
                if (v.StartsWith(' ')) v = v[1..];
                dataLines.Add(v);
            }
        }
        if (dataLines.Count == 0) return null;
        var data = string.Join("\n", dataLines);
        JsonElement parsed;
        try
        {
            parsed = JsonDocument.Parse(data).RootElement.Clone();
        }
        catch (JsonException)
        {
            parsed = default;
            // Keep raw string as JSON string element when not valid JSON.
            using var stringDoc = JsonDocument.Parse(JsonSerializer.Serialize(data));
            parsed = stringDoc.RootElement.Clone();
        }
        return new SseEventBlock { Id = id, Event = eventName, Data = parsed, HasIdLine = raw.Contains("id:") };
    }

    /// <summary>
    /// Subscribe to an SSE endpoint. Returns a dispose handle that cancels the stream.
    /// </summary>
    public IDisposable Subscribe(string path, string aggregateId, SseHandlers handlers, CancellationToken cancellationToken = default)
    {
        var lifetime = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        _ = Task.Run(() => RunAsync(path, aggregateId, handlers, lifetime.Token), CancellationToken.None);
        return lifetime;
    }

    private async Task RunAsync(string path, string aggregateId, SseHandlers handlers, CancellationToken ct)
    {
        var lastEventId = handlers.LastEventId != null && handlers.LastEventId.StartsWith(aggregateId + ":", StringComparison.Ordinal)
            ? handlers.LastEventId
            : "";
        var maxSeq = 0L;
        if (!string.IsNullOrEmpty(lastEventId))
        {
            var tail = lastEventId[(aggregateId.Length + 1)..];
            if (long.TryParse(tail, out var s)) maxSeq = s;
        }
        var retries = 0;
        try
        {
            while (!ct.IsCancellationRequested)
            {
                try
                {
                    using var request = new HttpRequestMessage(HttpMethod.Get, path);
                    request.Headers.Accept.ParseAdd("text/event-stream");
                    var token = TavernApiClient.AuthToken;
                    if (!string.IsNullOrEmpty(token))
                        request.Headers.TryAddWithoutValidation("Authorization", "Bearer " + token);
                    if (!string.IsNullOrEmpty(lastEventId))
                        request.Headers.TryAddWithoutValidation("Last-Event-ID", lastEventId);

                    using var connectCts = CancellationTokenSource.CreateLinkedTokenSource(ct);
                    connectCts.CancelAfter(StallTimeout);
                    using var response = await _http.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, connectCts.Token).ConfigureAwait(false);
                    if (!response.IsSuccessStatusCode)
                    {
                        var code = (int)response.StatusCode;
                        if (code is >= 400 and < 500 and not 429)
                        {
                            if (code == 401) TavernApiClient.NotifyAuthRequired();
                            handlers.OnFinalError(new ApiError(code, "SSE connect failed"));
                            return;
                        }
                        throw new HttpRequestException($"SSE temporarily unavailable ({code})");
                    }

                    if (retries > 0) handlers.OnReconnect?.Invoke(retries);

                    var stream = await response.Content.ReadAsStreamAsync(ct).ConfigureAwait(false);
                    var reader = new StreamReader(stream, Encoding.UTF8);
                    var buf = "";
                    var terminal = false;
                    while (!ct.IsCancellationRequested)
                    {
                        var chunkCts = CancellationTokenSource.CreateLinkedTokenSource(ct);
                        chunkCts.CancelAfter(StallTimeout);
                        string? line;
                        try
                        {
                            line = await reader.ReadLineAsync(chunkCts.Token).ConfigureAwait(false);
                        }
                        catch (OperationCanceledException) when (!ct.IsCancellationRequested)
                        {
                            throw new TimeoutException("SSE stalled");
                        }
                        finally
                        {
                            chunkCts.Dispose();
                        }
                        if (line is null) break;
                        buf += line + "\n";
                        if (buf.Length > 1024 * 1024) throw new InvalidOperationException("SSE event too large");
                        // Blank line flushes one event block.
                        if (line.Length != 0) continue;
                        var block = buf[..^1];
                        buf = "";
                        var evt = ParseSseBlock(block, lastEventId);
                        if (evt is null) continue;
                        // Unnumbered deltas inherit last-event-id but are not durable business events.
                        if (evt.HasIdLine)
                        {
                            if (!evt.Id.StartsWith(aggregateId + ":", StringComparison.Ordinal)) continue;
                            if (!long.TryParse(evt.Id[(aggregateId.Length + 1)..], out var seq) || seq <= maxSeq) continue;
                            maxSeq = seq;
                            lastEventId = evt.Id;
                        }
                        if (ct.IsCancellationRequested) return;
                        handlers.OnEvent(new SSEEvent { Id = evt.Id, Event = evt.Event, Data = evt.Data });
                        if (TerminalEvents.Contains(evt.Event))
                        {
                            terminal = true;
                            break;
                        }
                    }
                    if (terminal || ct.IsCancellationRequested) return;
                }
                catch (Exception) when (!ct.IsCancellationRequested)
                {
                    // fall through to backoff
                }
                if (ct.IsCancellationRequested) return;
                if (retries >= DefaultBackoff.Length)
                {
                    handlers.OnFinalError(new Exception("SSE reconnect exhausted"));
                    return;
                }
                var delay = DefaultBackoff[retries++];
                try { await Task.Delay(delay, ct).ConfigureAwait(false); }
                catch (OperationCanceledException) { return; }
            }
        }
        catch (Exception ex) when (!ct.IsCancellationRequested)
        {
            handlers.OnFinalError(ex);
        }
    }
}

public sealed class SseEventBlock
{
    public string Id { get; set; } = "";
    public string Event { get; set; } = "message";
    public JsonElement Data { get; set; }
    public bool HasIdLine { get; set; }
}
