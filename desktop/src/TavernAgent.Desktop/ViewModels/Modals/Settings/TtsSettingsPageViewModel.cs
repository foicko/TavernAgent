using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.Services;

namespace TavernAgent.Desktop.ViewModels.Modals.Settings;

/// <summary>Voice / TTS settings tab (web: TTSSettingsPage).</summary>
public sealed partial class TtsSettingsPageViewModel : WorkspaceChildViewModel, ISettingsPage
{
    private readonly TTSSettings _s;
    private readonly IDisposable _ttsSub;
    private bool _loaded;
    private bool _disposed;

    public TtsSettingsPageViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        _s = SettingsPersistence.LoadTts();
        _selectedEngine = Engines.FirstOrDefault(o => o.Value == Wire(_s.Engine)) ?? Engines[0];
        _edgeVoice = _s.EdgeVoice;
        _customBaseUrl = _s.CustomBaseUrl;
        _customApiKey = _s.CustomApiKey;
        _customModel = _s.CustomModel;
        _customVoice = _s.CustomVoice;
        _speed = _s.Speed;
        _dialogueOnly = _s.DialogueOnly;
        _directorMode = _s.DirectorMode;
        _loaded = true;
        _ttsSub = UiEvents.Event(
            h => AppServices.Tts.StateChanged += h,
            h => AppServices.Tts.StateChanged -= h,
            OnPlayerState);
    }

    public IReadOnlyList<OptionItem> Engines { get; } = new List<OptionItem>
    {
        new("edge", "Edge-TTS（微软在线，免费）"),
        new("openai", "OpenAI 兼容端点（本地/云端）"),
        new("mimo", "小米 MiMo TTS"),
    };

    public ObservableCollection<TTSVoice> Voices { get; } = new();

    public TTSVoice? SelectedVoice
    {
        get => Voices.FirstOrDefault(v => v.Id == EdgeVoice) ?? Voices.FirstOrDefault();
        set { if (value is not null) EdgeVoice = value.Id; }
    }

    [ObservableProperty] private OptionItem _selectedEngine;
    [ObservableProperty] private string _edgeVoice = "zh-CN-XiaoxiaoNeural";
    [ObservableProperty] private string _customBaseUrl = "";
    [ObservableProperty] private string _customApiKey = "";
    [ObservableProperty] private string _customModel = "";
    [ObservableProperty] private string _customVoice = "";
    [ObservableProperty] private double _speed = 1.0;
    [ObservableProperty] private bool _dialogueOnly = true;
    [ObservableProperty] private bool _directorMode = true;
    [ObservableProperty] private string? _testError;
    [ObservableProperty] private string? _notice;
    [ObservableProperty] private bool _playing;

    public bool IsEdge => SelectedEngine.Value == "edge";
    public bool IsCustom => SelectedEngine.Value is "openai" or "mimo";
    public bool IsWebSpeech => SelectedEngine.Value == "web-speech";
    public bool HasError => !string.IsNullOrEmpty(TestError);
    public bool HasNotice => !string.IsNullOrEmpty(Notice);
    public string SpeedLabel => $"{Speed:0.0}x";

    partial void OnSelectedEngineChanged(OptionItem value)
    {
        _s.Engine = Unwire(value.Value);
        Persist();
        OnPropertyChanged(nameof(IsEdge));
        OnPropertyChanged(nameof(IsCustom));
        OnPropertyChanged(nameof(IsWebSpeech));
    }

    partial void OnEdgeVoiceChanged(string value) { _s.EdgeVoice = value; Persist(); OnPropertyChanged(nameof(SelectedVoice)); }
    partial void OnCustomBaseUrlChanged(string value) { _s.CustomBaseUrl = value; Persist(); }
    partial void OnCustomApiKeyChanged(string value) { _s.CustomApiKey = value; Persist(); }
    partial void OnCustomModelChanged(string value) { _s.CustomModel = value; Persist(); }
    partial void OnCustomVoiceChanged(string value) { _s.CustomVoice = value; Persist(); }
    partial void OnSpeedChanged(double value) { _s.Speed = value; Persist(); OnPropertyChanged(nameof(SpeedLabel)); }
    partial void OnDialogueOnlyChanged(bool value) { _s.DialogueOnly = value; Persist(); }
    partial void OnDirectorModeChanged(bool value) { _s.DirectorMode = value; Persist(); }
    partial void OnTestErrorChanged(string? value) => OnPropertyChanged(nameof(HasError));
    partial void OnNoticeChanged(string? value) => OnPropertyChanged(nameof(HasNotice));

    public async Task LoadAsync()
    {
        if (Voices.Count > 0) return;
        try
        {
            var voices = await Workspace.Story.Api.GetTTSVoicesAsync().ConfigureAwait(true);
            foreach (var v in voices) Voices.Add(v);
        }
        catch
        {
            // fall through to the built-in defaults
        }
        if (Voices.Count == 0)
        {
            foreach (var v in FallbackVoices) Voices.Add(v);
        }
        OnPropertyChanged(nameof(SelectedVoice));
    }

    [RelayCommand]
    private void ApplyMimo()
    {
        SelectedEngine = Engines.First(o => o.Value == "mimo");
        CustomBaseUrl = "https://api.xiaomimimo.com/v1";
        CustomModel = "mimo-v2.5-tts";
        CustomVoice = "mimo_default";
        Notice = "已应用小米 MiMo 预设。";
    }

    [RelayCommand]
    private void ApplyLocal()
    {
        SelectedEngine = Engines.First(o => o.Value == "openai");
        CustomBaseUrl = "http://127.0.0.1:9880/v1";
        CustomApiKey = "";
        CustomModel = "tts-1";
        Notice = "已应用本地端点预设（GPT-SoVITS / CosyVoice 等）。";
    }

    [RelayCommand]
    private void ApplyOpenAI()
    {
        SelectedEngine = Engines.First(o => o.Value == "openai");
        CustomBaseUrl = "https://api.openai.com/v1";
        CustomModel = "tts-1";
        Notice = "已应用 OpenAI 端点预设。";
    }

    [RelayCommand]
    private void ResetDefaults()
    {
        var d = new TTSSettings();
        SelectedEngine = Engines.First(o => o.Value == Wire(d.Engine));
        EdgeVoice = d.EdgeVoice;
        CustomBaseUrl = d.CustomBaseUrl;
        CustomApiKey = d.CustomApiKey;
        CustomModel = d.CustomModel;
        CustomVoice = d.CustomVoice;
        Speed = d.Speed;
        DialogueOnly = d.DialogueOnly;
        DirectorMode = d.DirectorMode;
        Notice = "已恢复默认语音设置。";
    }

    [RelayCommand]
    private async Task TestPlaybackAsync()
    {
        TestError = null;
        Notice = null;
        Playing = true;
        try
        {
            await AppServices.Tts.ToggleAsync(BuildRequest("你好！很高兴能与你一同开启这段旅程。")).ConfigureAwait(true);
            if (AppServices.Tts.LastError is { } err) TestError = err;
        }
        catch (Exception ex)
        {
            TestError = "试听失败：" + ex.Message;
        }
        finally
        {
            Playing = AppServices.Tts.IsPlaying;
        }
    }

    public TtsPlayRequest BuildRequest(string text) => new()
    {
        Text = text,
        Voice = IsEdge ? EdgeVoice : CustomVoice,
        Engine = SelectedEngine.Value,
        BaseUrl = IsCustom ? CustomBaseUrl : null,
        ApiKey = IsCustom ? CustomApiKey : null,
        Model = IsCustom ? CustomModel : null,
        Speed = Speed,
    };

    private void OnPlayerState(object? sender, EventArgs e) => Playing = AppServices.Tts.IsPlaying;

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        _ttsSub.Dispose();
    }

    private void Persist() { if (_loaded) SettingsPersistence.SaveTts(_s); }

    private static string Wire(TTSEngine e) => e switch
    {
        TTSEngine.Openai => "openai",
        TTSEngine.Mimo => "mimo",
        TTSEngine.WebSpeech => "web-speech",
        _ => "edge",
    };

    private static TTSEngine Unwire(string value) => value switch
    {
        "openai" => TTSEngine.Openai,
        "mimo" => TTSEngine.Mimo,
        "web-speech" => TTSEngine.WebSpeech,
        _ => TTSEngine.Edge,
    };

    private static readonly IReadOnlyList<TTSVoice> FallbackVoices = new List<TTSVoice>
    {
        new() { Id = "zh-CN-XiaoxiaoNeural", Name = "晓晓 · 女声", Gender = "Female", Locale = "zh-CN" },
        new() { Id = "zh-CN-YunxiNeural", Name = "云希 · 男声", Gender = "Male", Locale = "zh-CN" },
        new() { Id = "zh-CN-YunyangNeural", Name = "云扬 · 男声", Gender = "Male", Locale = "zh-CN" },
        new() { Id = "zh-CN-XiaoyiNeural", Name = "晓伊 · 女声", Gender = "Female", Locale = "zh-CN" },
        new() { Id = "en-US-AriaNeural", Name = "Aria · Female", Gender = "Female", Locale = "en-US" },
        new() { Id = "en-US-GuyNeural", Name = "Guy · Male", Gender = "Male", Locale = "en-US" },
    };
}
