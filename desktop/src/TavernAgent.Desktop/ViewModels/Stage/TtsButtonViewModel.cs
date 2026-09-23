using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.Services;

namespace TavernAgent.Desktop.ViewModels.Stage;

/// <summary>Per-turn "read aloud" button (web: TTSPlayButton).</summary>
public sealed partial class TtsButtonViewModel : ObservableObject
{
    private readonly StageViewModel _stage;
    private readonly IDisposable _ttsSub;
    private TTSSettings _settings;

    public TtsButtonViewModel(StageViewModel stage, string fullText, string dialogueText)
    {
        _stage = stage;
        FullText = fullText;
        DialogueText = dialogueText;
        _settings = SettingsPersistence.LoadTts();
        _ttsSub = UiEvents.Event(
            h => AppServices.Tts.StateChanged += h,
            h => AppServices.Tts.StateChanged -= h,
            OnPlayerState);
    }

    public string FullText { get; }
    public string DialogueText { get; }

    public bool HasDialogue => !string.IsNullOrWhiteSpace(DialogueText);
    public bool IsPlaying => AppServices.Tts.IsPlayingText(Target());

    [ObservableProperty] private string? _notice;
    [ObservableProperty] private bool _showNoDialogue;

    public bool HasNotice => !string.IsNullOrEmpty(Notice);
    public string Label => IsPlaying ? "⏹ 停止" : (_settings.DialogueOnly ? "🔊 朗读" : "朗读正文");
    public string Tooltip => IsPlaying ? "停止朗读" : (_settings.DialogueOnly ? "朗读角色台词" : "朗读正文");

    partial void OnNoticeChanged(string? value) => OnPropertyChanged(nameof(HasNotice));

    [RelayCommand]
    private async Task PlayAsync()
    {
        _settings = SettingsPersistence.LoadTts();
        if (AppServices.Tts.IsPlayingText(Target()))
        {
            AppServices.Tts.Stop();
            RaiseState();
            return;
        }

        var text = _settings.DialogueOnly ? DialogueText : FullText;
        if (string.IsNullOrWhiteSpace(text))
        {
            ShowNoDialogue = true;
            Notice = "仅旁白";
            await Task.Delay(2200).ConfigureAwait(true);
            ShowNoDialogue = false;
            Notice = null;
            return;
        }

        var isMiMo = TtsDirector.IsMiMoEngine(Wire(_settings.Engine), _settings.CustomModel, _settings.CustomBaseUrl);
        var voice = _settings.Engine == TTSEngine.Edge ? _settings.EdgeVoice : _settings.CustomVoice;
        var request = new TtsPlayRequest
        {
            Text = text,
            Voice = voice,
            Engine = Wire(_settings.Engine),
            BaseUrl = _settings.Engine is TTSEngine.Openai or TTSEngine.Mimo ? _settings.CustomBaseUrl : null,
            ApiKey = _settings.Engine is TTSEngine.Openai or TTSEngine.Mimo ? _settings.CustomApiKey : null,
            Model = _settings.Engine is TTSEngine.Openai or TTSEngine.Mimo ? _settings.CustomModel : null,
            Speed = _settings.Speed,
        };

        if (_settings.DirectorMode && isMiMo)
        {
            request = request with
            {
                Text = TtsDirector.Enrich(text, null),
                Instruction = TtsDirector.BuildInstruction(_stage.CharName, _stage.SceneTitle, null),
            };
        }

        await AppServices.Tts.ToggleAsync(request).ConfigureAwait(true);
        if (AppServices.Tts.LastError is { } err) Notice = err;
        RaiseState();
    }

    private string Target() => _settings.DialogueOnly ? DialogueText : FullText;

    private void OnPlayerState(object? sender, EventArgs e) => RaiseState();

    private void RaiseState()
    {
        OnPropertyChanged(nameof(IsPlaying));
        OnPropertyChanged(nameof(Label));
        OnPropertyChanged(nameof(Tooltip));
    }

    public void Detach() => _ttsSub.Dispose();

    private static string Wire(TTSEngine e) => e switch
    {
        TTSEngine.Openai => "openai",
        TTSEngine.Mimo => "mimo",
        TTSEngine.WebSpeech => "web-speech",
        _ => "edge",
    };
}
