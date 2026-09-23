using SoundFlow.Abstracts.Devices;
using SoundFlow.Backends.MiniAudio;
using SoundFlow.Backends.MiniAudio.Devices;
using SoundFlow.Components;
using SoundFlow.Enums;
using SoundFlow.Providers;
using SoundFlow.Structs;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Desktop.Services;

public sealed record TtsPlayRequest
{
    public string Text { get; init; } = "";
    public string? Voice { get; init; }
    public string? Instruction { get; init; }
    public string? Engine { get; init; }
    public string? BaseUrl { get; init; }
    public string? ApiKey { get; init; }
    public string? Model { get; init; }
    public double? Speed { get; init; }
}

/// <summary>
/// Synthesizes (via the sidecar) and plays TTS audio through SoundFlow / miniaudio.
/// One shared playback device; a fresh player per utterance; text-keyed audio cache.
/// </summary>
public sealed class TtsPlayer : IDisposable
{
    private readonly Func<TtsPlayRequest, CancellationToken, Task<byte[]>> _synthesize;
    private readonly object _gate = new();
    private readonly Dictionary<string, byte[]> _cache = new();

    private MiniAudioEngine? _engine;
    private AudioPlaybackDevice? _device;
    private SoundPlayer? _player;
    private StreamDataProvider? _provider;
    private MemoryStream? _stream;
    private System.Threading.Timer? _poll;
    private string? _currentText;
    private string? _lastError;

    public TtsPlayer(Func<TtsPlayRequest, CancellationToken, Task<byte[]>> synthesize)
    {
        _synthesize = synthesize;
    }

    public event EventHandler? StateChanged;

    public bool IsPlaying
    {
        get { lock (_gate) return _player is not null && _player.State == PlaybackState.Playing; }
    }

    public string? CurrentText { get { lock (_gate) return _currentText; } }

    public string? LastError { get { lock (_gate) return _lastError; } }

    public bool IsPlayingText(string text) => IsPlaying && (_currentText == text || (_currentText ?? "").Contains(text));

    /// <summary>Toggle: play the given utterance, or stop if the same text is already playing.</summary>
    public async Task ToggleAsync(TtsPlayRequest request, CancellationToken ct = default)
    {
        var text = request.Text.Trim();
        if (string.IsNullOrEmpty(text)) return;

        if (IsPlayingText(text))
        {
            Stop();
            return;
        }

        Stop();
        byte[] audio;
        try
        {
            var key = CacheKey(request);
            byte[]? cached;
            lock (_gate) _cache.TryGetValue(key, out cached);
            if (cached is not null)
            {
                audio = cached;
            }
            else
            {
                audio = await _synthesize(request, ct).ConfigureAwait(true);
                if (audio is null || audio.Length == 0) throw new Exception("语音合成返回空数据");
                lock (_gate)
                {
                    if (_cache.Count > 24) _cache.Clear();
                    _cache[key] = audio;
                }
            }
        }
        catch (Exception ex)
        {
            lock (_gate) _lastError = ex.Message;
            RaiseStateChanged();
            return;
        }

        try
        {
            StartPlayback(audio, text);
            lock (_gate) _lastError = null;
        }
        catch (Exception ex)
        {
            lock (_gate) _lastError = "播放失败：" + ex.Message;
            Stop();
        }
        RaiseStateChanged();
    }

    /// <summary>
    /// Always raised on the UI thread: subscribers (TTS buttons/settings) push
    /// bindable state, and the playback poll timer fires from a thread-pool thread.
    /// </summary>
    private void RaiseStateChanged()
        => UiDispatcher.Post(() => StateChanged?.Invoke(this, EventArgs.Empty));

    private void StartPlayback(byte[] audio, string text)
    {
        // Everything here touches native miniaudio objects that Stop()/Dispose()
        // may free from the poll timer thread; hold the gate for the whole build.
        lock (_gate)
        {
            var engine = EnsureEngine();
            AudioFormat format;
            using (var probe = new MemoryStream(audio))
                format = AudioFormat.GetFormatFromStream(probe) ?? AudioFormat.Cd;
            var device = EnsureDevice(format);

            _stream = new MemoryStream(audio);
            _provider = new StreamDataProvider(engine, format, _stream);
            _player = new SoundPlayer(engine, format, _provider);
            device.MasterMixer.AddComponent(_player);
            _player.Play();
            _currentText = text;

            _poll?.Dispose();
            _poll = new System.Threading.Timer(_ => PollPlayback(), null, 400, 400);
        }
    }

    private void PollPlayback()
    {
        bool stopped;
        lock (_gate) stopped = _player is null || _player.State != PlaybackState.Playing;
        if (!stopped) return;
        Stop();
        RaiseStateChanged();
    }

    public void Stop()
    {
        lock (_gate)
        {
            _poll?.Dispose();
            _poll = null;
            try { _player?.Stop(); } catch { /* ignore */ }
            if (_player is not null && _device is not null)
            {
                try { _device.MasterMixer.RemoveComponent(_player); } catch { /* ignore */ }
            }
            try { _player?.Dispose(); } catch { /* ignore */ }
            try { _provider?.Dispose(); } catch { /* ignore */ }
            try { _stream?.Dispose(); } catch { /* ignore */ }
            _player = null;
            _provider = null;
            _stream = null;
            _currentText = null;
        }
    }

    private MiniAudioEngine EnsureEngine()
    {
        if (_engine is not null) return _engine;
        _engine = new MiniAudioEngine(MiniAudioEngine.AvailableBackends);
        return _engine;
    }

    private AudioPlaybackDevice EnsureDevice(AudioFormat format)
    {
        if (_device is not null) return _device;
        _device = _engine!.InitializePlaybackDevice(null, format, new MiniAudioDeviceConfig());
        return _device;
    }

    private static string CacheKey(TtsPlayRequest r)
        => $"{r.Engine}:{r.Voice}:{r.Speed}:{r.BaseUrl}:{r.Instruction}:{r.Text}";

    public void Dispose()
    {
        Stop();
        lock (_gate)
        {
            try { _device?.Dispose(); } catch { /* ignore */ }
            try { _engine?.Dispose(); } catch { /* ignore */ }
            _device = null;
            _engine = null;
        }
    }
}
