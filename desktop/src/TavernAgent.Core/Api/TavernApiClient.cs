using System.Net;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Core.Api;

/// <summary>
/// REST client parity with web/src/app/api.ts (auth token, 30s timeout, binary paths).
/// </summary>
public sealed class TavernApiClient : IDisposable
{
    public const string AuthStorageFileName = "auth_token";

    private static readonly HashSet<string> AuthListenersGate = new();
    private static readonly object AuthGateLock = new();
    private static event Action? AuthRequired;

    private readonly HttpClient _http;
    private readonly SseClient _sse;
    private readonly bool _ownsHttp;
    private string? _authTokenPath;

    public TavernApiClient(Uri? baseAddress, HttpClient? http = null, string? authTokenPath = null)
    {
        if (http is not null)
        {
            _http = http;
            _ownsHttp = false;
        }
        else
        {
            _http = new HttpClient { Timeout = TimeSpan.FromSeconds(30) };
            _ownsHttp = true;
        }
        if (baseAddress is not null) _http.BaseAddress = baseAddress;
        _sse = new SseClient(_http);
        _authTokenPath = authTokenPath ?? DefaultAuthTokenPath();
    }

    public Uri? BaseAddress
    {
        get => _http.BaseAddress;
        set => _http.BaseAddress = value;
    }

    public HttpClient Http => _http;

    public static string? AuthToken { get; private set; }

    public static void SetAuthToken(string token)
    {
        AuthToken = string.IsNullOrEmpty(token) ? null : token;
        PersistAuthToken(AuthToken);
    }

    public static void ClearAuthToken() => SetAuthToken("");

    public static IDisposable OnAuthRequired(Action fn)
    {
        AuthRequired += fn;
        return new Unsubscriber(() => AuthRequired -= fn);
    }

    internal static void NotifyAuthRequired()
    {
        try { AuthRequired?.Invoke(); }
        catch { /* listeners must not break the pipeline */ }
    }

    public static void LoadAuthToken(string? path = null)
    {
        try
        {
            path ??= DefaultAuthTokenPath();
            if (File.Exists(path))
            {
                var token = File.ReadAllText(path).Trim();
                AuthToken = string.IsNullOrEmpty(token) ? null : token;
                return;
            }
        }
        catch { /* privacy modes / IO failures ignore */ }
        AuthToken = null;
    }

    private static void PersistAuthToken(string? token)
    {
        try
        {
            var path = DefaultAuthTokenPath();
            var dir = Path.GetDirectoryName(path);
            if (!string.IsNullOrEmpty(dir)) Directory.CreateDirectory(dir);
            if (token is null)
            {
                if (File.Exists(path)) File.Delete(path);
            }
            else
            {
                File.WriteAllText(path, token);
            }
        }
        catch { /* ignore */ }
    }

    private static string DefaultAuthTokenPath()
    {
        var root = Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData);
        if (string.IsNullOrEmpty(root)) root = Path.GetTempPath();
        return Path.Combine(root, "TavernAgent", AuthStorageFileName);
    }

    private sealed class Unsubscriber(Action dispose) : IDisposable
    {
        private Action? _dispose = dispose;
        public void Dispose() => Interlocked.Exchange(ref _dispose, null)?.Invoke();
    }

    // ---- core HTTP ----

    private async Task<T> HttpAsync<T>(string method, string path, object? body, CancellationToken ct = default)
    {
        using var req = new HttpRequestMessage(new HttpMethod(method), path);
        if (body is not null)
        {
            req.Content = new StringContent(JsonSerializer.Serialize(body, body.GetType(), JsonDefaults.Options), Encoding.UTF8, "application/json");
        }
        ApplyAuth(req);
        using var cts = CancellationTokenSource.CreateLinkedTokenSource(ct);
        cts.CancelAfter(TimeSpan.FromSeconds(30));
        HttpResponseMessage res;
        try
        {
            res = await _http.SendAsync(req, cts.Token).ConfigureAwait(false);
        }
        catch (OperationCanceledException) when (!ct.IsCancellationRequested)
        {
            throw new ApiError(0, "请求超时，请检查网络或稍后重试");
        }
        using (res)
        {
            var text = await res.Content.ReadAsStringAsync(cts.Token).ConfigureAwait(false);
            if (!res.IsSuccessStatusCode)
            {
                if (res.StatusCode == HttpStatusCode.Unauthorized)
                {
                    try
                    {
                        using var doc = JsonDocument.Parse(text);
                        if (doc.RootElement.ValueKind == JsonValueKind.Object &&
                            doc.RootElement.TryGetProperty("code", out var code) &&
                            code.GetString() == "AUTH_REQUIRED")
                        {
                            NotifyAuthRequired();
                        }
                    }
                    catch (JsonException) { /* non-JSON body */ }
                }
                throw new ApiError((int)res.StatusCode, text);
            }
            if (string.IsNullOrEmpty(text)) return default!;
            return JsonSerializer.Deserialize<T>(text, JsonDefaults.Options)!;
        }
    }

    private void ApplyAuth(HttpRequestMessage req)
    {
        var token = AuthToken;
        if (!string.IsNullOrEmpty(token))
            req.Headers.TryAddWithoutValidation("Authorization", "Bearer " + token);
    }

    private async Task<byte[]> BinaryRequestAsync(string path, HttpContent? content, CancellationToken ct = default)
    {
        using var req = new HttpRequestMessage(HttpMethod.Post, path) { Content = content };
        ApplyAuth(req);
        using var cts = CancellationTokenSource.CreateLinkedTokenSource(ct);
        cts.CancelAfter(TimeSpan.FromSeconds(30));
        HttpResponseMessage res;
        try
        {
            res = await _http.SendAsync(req, cts.Token).ConfigureAwait(false);
        }
        catch (OperationCanceledException) when (!ct.IsCancellationRequested)
        {
            throw new ApiError(0, "请求超时，请检查网络或稍后重试");
        }
        using (res)
        {
            if (!res.IsSuccessStatusCode)
            {
                var text = await res.Content.ReadAsStringAsync(cts.Token).ConfigureAwait(false);
                if (res.StatusCode == HttpStatusCode.Unauthorized) NotifyAuthRequired();
                throw new ApiError((int)res.StatusCode, text);
            }
            return await res.Content.ReadAsByteArrayAsync(cts.Token).ConfigureAwait(false);
        }
    }

    private async Task<T> BinaryJsonAsync<T>(string path, HttpContent? content, HttpMethod? method = null, CancellationToken ct = default)
    {
        using var req = new HttpRequestMessage(method ?? HttpMethod.Post, path) { Content = content };
        ApplyAuth(req);
        using var cts = CancellationTokenSource.CreateLinkedTokenSource(ct);
        cts.CancelAfter(TimeSpan.FromSeconds(30));
        HttpResponseMessage res;
        try
        {
            res = await _http.SendAsync(req, cts.Token).ConfigureAwait(false);
        }
        catch (OperationCanceledException) when (!ct.IsCancellationRequested)
        {
            throw new ApiError(0, "请求超时，请检查网络或稍后重试");
        }
        using (res)
        {
            var text = await res.Content.ReadAsStringAsync(cts.Token).ConfigureAwait(false);
            if (!res.IsSuccessStatusCode)
            {
                if (res.StatusCode == HttpStatusCode.Unauthorized) NotifyAuthRequired();
                throw new ApiError((int)res.StatusCode, text);
            }
            if (string.IsNullOrEmpty(text)) return default!;
            return JsonSerializer.Deserialize<T>(text, JsonDefaults.Options)!;
        }
    }

    // ---- auth ----

    public Task<AuthStatus> AuthStatusAsync(CancellationToken ct = default)
        => HttpAsync<AuthStatus>(HttpMethod.Get.Method, "/api/v1/auth/status", null, ct);

    public Task<PairResult> PairAsync(string pin, CancellationToken ct = default)
        => HttpAsync<PairResult>(HttpMethod.Post.Method, "/api/v1/auth/pair", new Dictionary<string, string> { ["pin"] = pin }, ct);

    public Task<DesktopInfo> DesktopInfoAsync(CancellationToken ct = default)
        => HttpAsync<DesktopInfo>(HttpMethod.Get.Method, "/api/v1/desktop", null, ct);

    public Task SetDesktopThemeAsync(string mode, CancellationToken ct = default)
        => HttpAsync<object?>(HttpMethod.Post.Method, "/api/v1/desktop/theme", new ThemeRequest { Mode = mode }, ct);

    // ---- sessions ----

    public async Task<List<SessionSummary>> ListSessionsAsync(CancellationToken ct = default)
    {
        var res = await HttpAsync<Dictionary<string, List<SessionSummary>>>(HttpMethod.Get.Method, "/api/v1/sessions", null, ct).ConfigureAwait(false);
        return res.TryGetValue("sessions", out var list) ? list : new List<SessionSummary>();
    }

    public Task<OkResult> DeleteSessionAsync(string id, CancellationToken ct = default)
        => HttpAsync<OkResult>(HttpMethod.Delete.Method, $"/api/v1/sessions/{Uri.EscapeDataString(id)}", null, ct);

    public async Task<SessionView> GetSessionAsync(
        string id,
        string? branchId = null,
        string? viewNodeId = null,
        int? limit = null,
        string? before = null,
        CancellationToken ct = default)
    {
        var qs = new List<string>();
        if (!string.IsNullOrEmpty(branchId)) qs.Add("branchId=" + Uri.EscapeDataString(branchId));
        if (!string.IsNullOrEmpty(viewNodeId)) qs.Add("viewNodeId=" + Uri.EscapeDataString(viewNodeId));
        if (limit is > 0) qs.Add("limit=" + limit.Value);
        if (!string.IsNullOrEmpty(before)) qs.Add("before=" + Uri.EscapeDataString(before));
        var path = $"/api/v1/sessions/{Uri.EscapeDataString(id)}" + (qs.Count > 0 ? "?" + string.Join("&", qs) : "");
        return await HttpAsync<SessionView>(HttpMethod.Get.Method, path, null, ct).ConfigureAwait(false);
    }

    public async Task<GraphView> GetGraphAsync(string sessionId, string? nodeId = null, int? up = null, int? down = null, CancellationToken ct = default)
    {
        var qs = new List<string>();
        if (!string.IsNullOrEmpty(nodeId)) qs.Add("nodeId=" + Uri.EscapeDataString(nodeId));
        if (up is not null) qs.Add("up=" + up.Value);
        if (down is not null) qs.Add("down=" + down.Value);
        var path = $"/api/v1/sessions/{Uri.EscapeDataString(sessionId)}/graph" + (qs.Count > 0 ? "?" + string.Join("&", qs) : "");
        return await HttpAsync<GraphView>(HttpMethod.Get.Method, path, null, ct).ConfigureAwait(false);
    }

    public async Task<LorebookPage> SearchLorebookAsync(string sessionId, string text, int offset = 0, CancellationToken ct = default)
    {
        var path = $"/api/v1/sessions/{Uri.EscapeDataString(sessionId)}/lorebook?q={Uri.EscapeDataString(text)}&offset={offset}&limit=40";
        return await HttpAsync<LorebookPage>(HttpMethod.Get.Method, path, null, ct).ConfigureAwait(false);
    }

    public Task<NodeResult> GetNodeAsync(string nodeId, CancellationToken ct = default)
        => HttpAsync<NodeResult>(HttpMethod.Get.Method, $"/api/v1/nodes/{Uri.EscapeDataString(nodeId)}", null, ct);

    public Task<CreateSessionResult> CreateSessionAsync(CreateSessionRequest req, CancellationToken ct = default)
        => HttpAsync<CreateSessionResult>(HttpMethod.Post.Method, "/api/v1/sessions", req, ct);

    public Task<AcceptResult> AcceptTurnAsync(string sessionId, string branchId, AcceptTurnRequest req, CancellationToken ct = default)
        => HttpAsync<AcceptResult>(HttpMethod.Post.Method, $"/api/v1/sessions/{Uri.EscapeDataString(sessionId)}/branches/{Uri.EscapeDataString(branchId)}/turns", req, ct);

    public Task<TurnView> GetTurnAsync(string turnId, CancellationToken ct = default)
        => HttpAsync<TurnView>(HttpMethod.Get.Method, $"/api/v1/turns/{Uri.EscapeDataString(turnId)}", null, ct);

    public Task<TurnStatusResult> CancelTurnAsync(string turnId, string? expectedCharacterId, CancellationToken ct = default)
        => HttpAsync<TurnStatusResult>(HttpMethod.Post.Method, $"/api/v1/turns/{Uri.EscapeDataString(turnId)}/cancel",
            new Dictionary<string, string?> { ["expectedCharacterId"] = expectedCharacterId }, ct);

    public Task<TurnStatusResult> ContinueTurnAsync(string turnId, string? expectedCharacterId, CancellationToken ct = default)
        => HttpAsync<TurnStatusResult>(HttpMethod.Post.Method, $"/api/v1/turns/{Uri.EscapeDataString(turnId)}/continue",
            new Dictionary<string, string?> { ["expectedCharacterId"] = expectedCharacterId }, ct);

    // ---- models ----

    public async Task<List<ProviderConfig>> GetProviderConfigsAsync(CancellationToken ct = default)
    {
        var res = await HttpAsync<Dictionary<string, List<ProviderConfig>>>(HttpMethod.Get.Method, "/api/v1/config/provider", null, ct).ConfigureAwait(false);
        return res.TryGetValue("providers", out var list) ? list : new List<ProviderConfig>();
    }

    public Task<ModelCatalog> ListModelsAsync(CancellationToken ct = default)
        => HttpAsync<ModelCatalog>(HttpMethod.Get.Method, "/api/v1/config/models", null, ct);

    public Task<ModelInstance> SaveModelAsync(ModelInstance instance, CancellationToken ct = default)
    {
        var method = string.IsNullOrEmpty(instance.Id) ? HttpMethod.Post : HttpMethod.Put;
        var path = string.IsNullOrEmpty(instance.Id)
            ? "/api/v1/config/models"
            : $"/api/v1/config/models/{Uri.EscapeDataString(instance.Id)}";
        return HttpAsync<ModelInstance>(method.Method, path, instance, ct);
    }

    public Task<OkResult> DeleteModelAsync(string id, CancellationToken ct = default)
        => HttpAsync<OkResult>(HttpMethod.Delete.Method, $"/api/v1/config/models/{Uri.EscapeDataString(id)}", null, ct);

    public async Task<List<ProviderConfig>> AssignSlotAsync(string slot, bool enabled, string modelId, CancellationToken ct = default)
    {
        var res = await HttpAsync<OkProvidersResult>(HttpMethod.Put.Method, "/api/v1/config/provider",
            new AssignSlotRequest { Slot = slot, Enabled = enabled, ModelId = modelId }, ct).ConfigureAwait(false);
        return res.Providers;
    }

    public Task<ProbeResult> ProbeProviderAsync(string slot, bool format = false, CancellationToken ct = default)
        => HttpAsync<ProbeResult>(HttpMethod.Post.Method, "/api/v1/config/provider/probe",
            new Dictionary<string, object> { ["slot"] = slot, ["format"] = format }, ct);

    public Task<ProbeResult> ProbeModelAsync(string modelId, bool format = false, CancellationToken ct = default)
        => HttpAsync<ProbeResult>(HttpMethod.Post.Method, $"/api/v1/config/models/{Uri.EscapeDataString(modelId)}/probe",
            new Dictionary<string, object> { ["format"] = format }, ct);

    // ---- branch derive ----

    public Task<ForkBranchResult> ForkBranchAsync(string sessionId, ForkBranchRequest req, CancellationToken ct = default)
        => HttpAsync<ForkBranchResult>(HttpMethod.Post.Method, $"/api/v1/sessions/{Uri.EscapeDataString(sessionId)}/branches", req, ct);

    public Task<DeriveResult> RegenerateTurnAsync(string sessionId, string branchId, RegenerateRequest req, CancellationToken ct = default)
        => HttpAsync<DeriveResult>(HttpMethod.Post.Method, $"/api/v1/sessions/{Uri.EscapeDataString(sessionId)}/branches/{Uri.EscapeDataString(branchId)}/regenerations", req, ct);

    public async Task<string> EditTurnAsync(string sessionId, string branchId, EditTurnRequest req, CancellationToken ct = default)
        => await HttpAsyncRawAsync(HttpMethod.Post.Method, $"/api/v1/sessions/{Uri.EscapeDataString(sessionId)}/branches/{Uri.EscapeDataString(branchId)}/edits", req, ct).ConfigureAwait(false);

    private async Task<string> HttpAsyncRawAsync(string method, string path, object? body, CancellationToken ct)
    {
        using var reqMsg = new HttpRequestMessage(new HttpMethod(method), path);
        if (body is not null)
        {
            reqMsg.Content = new StringContent(System.Text.Json.JsonSerializer.Serialize(body, body.GetType(), JsonDefaults.Options), System.Text.Encoding.UTF8, "application/json");
        }
        ApplyAuth(reqMsg);
        using var cts = CancellationTokenSource.CreateLinkedTokenSource(ct);
        cts.CancelAfter(TimeSpan.FromSeconds(30));
        using var res = await _http.SendAsync(reqMsg, cts.Token).ConfigureAwait(false);
        var text = await res.Content.ReadAsStringAsync(cts.Token).ConfigureAwait(false);
        if (!res.IsSuccessStatusCode)
        {
            if (res.StatusCode == System.Net.HttpStatusCode.Unauthorized) NotifyAuthRequired();
            throw new ApiError((int)res.StatusCode, text);
        }
        return text;
    }

    // ---- packs / cards ----

    public async Task<byte[]> ExportSessionPackAsync(string sessionId, string? branchId = null, CancellationToken ct = default)
    {
        var path = $"/api/v1/sessions/{Uri.EscapeDataString(sessionId)}/export" +
                   (string.IsNullOrEmpty(branchId) ? "" : "?branchId=" + Uri.EscapeDataString(branchId));
        return await BinaryRequestAsync(path, null, ct).ConfigureAwait(false);
    }

    public async Task<ImportResult> ImportSessionPackAsync(byte[] data, CancellationToken ct = default)
    {
        using var content = new ByteArrayContent(data);
        content.Headers.ContentType = new MediaTypeHeaderValue("application/zip");
        return await BinaryJsonAsync<ImportResult>("/api/v1/sessions/import", content, ct: ct).ConfigureAwait(false);
    }

    public async Task<CardPreview> ImportCardFileAsync(string fileName, byte[] data, string contentType = "application/octet-stream", CancellationToken ct = default)
    {
        using var form = new MultipartFormDataContent();
        var file = new ByteArrayContent(data);
        file.Headers.ContentType = new MediaTypeHeaderValue(contentType);
        form.Add(file, "file", fileName);
        return await BinaryJsonAsync<CardPreview>("/api/v1/cards", form, ct: ct).ConfigureAwait(false);
    }

    public async Task<List<CardLibrarySummary>> ListCardsAsync(CancellationToken ct = default)
    {
        var res = await HttpAsync<Dictionary<string, List<CardLibrarySummary>>>(HttpMethod.Get.Method, "/api/v1/cards", null, ct).ConfigureAwait(false);
        return res.TryGetValue("cards", out var list) ? list : new List<CardLibrarySummary>();
    }

    public Task<CardPreview> GetCardAsync(string cardId, CancellationToken ct = default)
        => HttpAsync<CardPreview>(HttpMethod.Get.Method, $"/api/v1/cards/{Uri.EscapeDataString(cardId)}", null, ct);

    public Task<OkResult> DeleteCardAsync(string cardId, CancellationToken ct = default)
        => HttpAsync<OkResult>(HttpMethod.Delete.Method, $"/api/v1/cards/{Uri.EscapeDataString(cardId)}", null, ct);

    // ---- memories ----

    public async Task<MemoryPage> ListMemoriesAsync(
        string sessionId,
        string? branchId = null,
        string? nodeId = null,
        string? search = null,
        string? kind = null,
        string? cursor = null,
        int limit = 40,
        CancellationToken ct = default)
    {
        var qs = new List<string> { "limit=" + limit };
        if (!string.IsNullOrEmpty(branchId)) qs.Add("branchId=" + Uri.EscapeDataString(branchId));
        if (!string.IsNullOrEmpty(nodeId)) qs.Add("nodeId=" + Uri.EscapeDataString(nodeId));
        if (!string.IsNullOrEmpty(search)) qs.Add("search=" + Uri.EscapeDataString(search));
        if (!string.IsNullOrEmpty(kind)) qs.Add("kind=" + Uri.EscapeDataString(kind));
        if (!string.IsNullOrEmpty(cursor)) qs.Add("cursor=" + Uri.EscapeDataString(cursor));
        var path = $"/api/v1/sessions/{Uri.EscapeDataString(sessionId)}/memories?{string.Join("&", qs)}";
        return await HttpAsync<MemoryPage>(HttpMethod.Get.Method, path, null, ct).ConfigureAwait(false);
    }

    public Task<MemoryView> PatchMemoryAsync(string sessionId, string branchId, string memoryId, PatchMemoryRequest patch, CancellationToken ct = default)
        => HttpAsync<MemoryView>(HttpMethod.Patch.Method,
            $"/api/v1/sessions/{Uri.EscapeDataString(sessionId)}/branches/{Uri.EscapeDataString(branchId)}/memories/{Uri.EscapeDataString(memoryId)}",
            patch, ct);

    public async Task<List<object>> OrganizeMemoriesAsync(string sessionId, string branchId, string expectedCharacterId, CancellationToken ct = default)
    {
        var res = await HttpAsync<OrganizeMemoriesResult>(HttpMethod.Post.Method,
            $"/api/v1/sessions/{Uri.EscapeDataString(sessionId)}/branches/{Uri.EscapeDataString(branchId)}/memories/organize",
            new OrganizeMemoriesRequest { ExpectedCharacterId = expectedCharacterId }, ct).ConfigureAwait(false);
        return res.Merged ?? new List<object>();
    }

    // ---- director ----

    public async Task<DirectorView> GetDirectorAsync(string sessionId, string branchId, string? viewNodeId = null, CancellationToken ct = default)
    {
        var path = $"/api/v1/sessions/{Uri.EscapeDataString(sessionId)}/branches/{Uri.EscapeDataString(branchId)}/director" +
                   (string.IsNullOrEmpty(viewNodeId) ? "" : "?viewNodeId=" + Uri.EscapeDataString(viewNodeId));
        return await HttpAsync<DirectorView>(HttpMethod.Get.Method, path, null, ct).ConfigureAwait(false);
    }

    public Task<DirectorDraft> SaveDirectorDraftAsync(string sessionId, string branchId, SaveDraftRequest req, CancellationToken ct = default)
        => HttpAsync<DirectorDraft>(HttpMethod.Put.Method,
            $"/api/v1/sessions/{Uri.EscapeDataString(sessionId)}/branches/{Uri.EscapeDataString(branchId)}/director/draft", req, ct);

    public Task<DirectorCommandResult> DirectorCommandAsync(string sessionId, string branchId, DirectorCommandRequest req, CancellationToken ct = default)
        => HttpAsync<DirectorCommandResult>(HttpMethod.Post.Method,
            $"/api/v1/sessions/{Uri.EscapeDataString(sessionId)}/branches/{Uri.EscapeDataString(branchId)}/director/commands", req, ct);

    public Task<DirectorMessageResult> DirectorMessageAsync(string sessionId, string branchId, DirectorMessageRequest req, CancellationToken ct = default)
        => HttpAsync<DirectorMessageResult>(HttpMethod.Post.Method,
            $"/api/v1/sessions/{Uri.EscapeDataString(sessionId)}/branches/{Uri.EscapeDataString(branchId)}/director/messages", req, ct);

    public Task<DirectorRequest> GetDirectorRequestAsync(string requestId, CancellationToken ct = default)
        => HttpAsync<DirectorRequest>(HttpMethod.Get.Method, $"/api/v1/director-requests/{Uri.EscapeDataString(requestId)}", null, ct);

    public Task<DirectorRequest> CancelDirectorRequestAsync(string requestId, string expectedCharacterId, CancellationToken ct = default)
        => HttpAsync<DirectorRequest>(HttpMethod.Post.Method, $"/api/v1/director-requests/{Uri.EscapeDataString(requestId)}/cancel",
            new CancelDirectorRequest { ExpectedCharacterId = expectedCharacterId }, ct);

    // ---- SSE ----

    public IDisposable SubscribeTurnEvents(string turnId, SseHandlers handlers, CancellationToken ct = default)
        => _sse.Subscribe($"/api/v1/turns/{Uri.EscapeDataString(turnId)}/events", turnId, handlers, ct);

    public IDisposable SubscribeSessionEvents(string sessionId, SseHandlers handlers, CancellationToken ct = default)
        => _sse.Subscribe($"/api/v1/sessions/{Uri.EscapeDataString(sessionId)}/events", sessionId, handlers, ct);

    public IDisposable SubscribeDirectorEvents(string requestId, SseHandlers handlers, CancellationToken ct = default)
        => _sse.Subscribe($"/api/v1/director-requests/{Uri.EscapeDataString(requestId)}/events", requestId, handlers, ct);

    // ---- tts / images / embeddings ----

    public async Task<List<TTSVoice>> GetTTSVoicesAsync(CancellationToken ct = default)
    {
        try
        {
            var res = await HttpAsync<TTSVoicesResult>(HttpMethod.Get.Method, "/api/v1/tts/voices", null, ct).ConfigureAwait(false);
            return res.Voices ?? new List<TTSVoice>();
        }
        catch
        {
            return new List<TTSVoice>();
        }
    }

    public async Task<byte[]> SynthesizeTTSAsync(TTSSynthesizeParams body, CancellationToken ct = default)
    {
        using var content = new StringContent(JsonSerializer.Serialize(body, JsonDefaults.Options), Encoding.UTF8, "application/json");
        return await BinaryRequestAsync("/api/v1/tts/synthesize", content, ct).ConfigureAwait(false);
    }

    public Task<GeneratedImageResult> GenerateImageAsync(GenerateImageParams body, CancellationToken ct = default)
        => HttpAsync<GeneratedImageResult>(HttpMethod.Post.Method, "/api/v1/images/generate", body, ct);

    public Task<EmbeddingProbeResponse> ProbeEmbeddingAsync(EmbeddingProbeRequest body, CancellationToken ct = default)
        => HttpAsync<EmbeddingProbeResponse>(HttpMethod.Post.Method, "/api/v1/embeddings/probe", body, ct);

    public Task<EmbeddingComputeResponse> ComputeEmbeddingsAsync(EmbeddingComputeRequest body, CancellationToken ct = default)
        => HttpAsync<EmbeddingComputeResponse>(HttpMethod.Post.Method, "/api/v1/embeddings", body, ct);

    public void Dispose()
    {
        if (_ownsHttp) _http.Dispose();
    }
}
