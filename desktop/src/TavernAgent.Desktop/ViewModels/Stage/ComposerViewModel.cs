using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Stage;

public sealed record MacroPill(string Label, string Text, string Prefix = "", string Suffix = "");

public sealed record ModelQuickItem(string Id, string Name, bool IsPrimary);

public sealed record EffortItem(string Value, string Label);

/// <summary>Floating-island prompt bar (web: ComposerShelf).</summary>
public sealed partial class ComposerViewModel : ObservableObject
{
    private readonly StageViewModel _stage;

    public ComposerViewModel(StageViewModel stage)
    {
        _stage = stage;
        Macros = new[]
        {
            new MacroPill("「」对白", "「」", "「", "」"),
            new MacroPill("﹡动作", "**", "*", "*"),
            new MacroPill("（）心声", "（）", "（", "）"),
        };
        if (_stage.Director is not null)
        {
            _stage.Director.PropertyChanged += (_, e) =>
            {
                if (e.PropertyName == nameof(DirectorViewModel.IsActive))
                {
                    OnPropertyChanged(nameof(IsDirectorModeBadgeVisible));
                    OnPropertyChanged(nameof(IsFreeformModeBadgeVisible));
                }
            };
        }
        Refresh();
    }

    [ObservableProperty] private string _text = "";
    [ObservableProperty] private string _note = "";
    [ObservableProperty] private bool _isNoteOpen;
    [ObservableProperty] private ModelQuickItem? _selectedModel;
    [ObservableProperty] private EffortItem? _selectedEffortItem;

    private bool _suppressSelection;

    public ContextMetricsViewModel Context => _stage.Context;

    public DirectorViewModel Director => _stage.Director;

    [RelayCommand]
    private void OpenDirector() => _stage.OpenDirector();

    public ObservableCollection<ModelQuickItem> Models { get; } = new();
    public IReadOnlyList<MacroPill> Macros { get; }
    public IReadOnlyList<EffortItem> Efforts { get; } = new[]
    {
        new EffortItem("", "默认"),
        new EffortItem("low", "低"),
        new EffortItem("medium", "中"),
        new EffortItem("high", "高"),
    };

    public bool HasNote => !string.IsNullOrWhiteSpace(Note);
    public bool HasModels => Models.Count > 0;

    public bool IsDirectorModeBadgeVisible => _stage.Director?.IsActive == true;
    public bool IsFreeformModeBadgeVisible => _stage.Director?.IsActive != true;

    public string Placeholder => _stage.Story.View is null
        ? "导入角色卡或打开一段会话即可开始推演…"
        : "输入你的行动或对白…";

    public bool CanSend => !_stage.Busy && !_stage.IsHistoryView && !string.IsNullOrWhiteSpace(Text);
    public bool CanCancel => _stage.Story.Phase == TurnPhase.Generating;
    public bool CanContinue => _stage.Story.Phase == TurnPhase.Truncated;
    public bool CanRetry => _stage.Story.Phase == TurnPhase.Failed;
    public bool ShowSendButton => !CanCancel && !CanContinue;
    public string SendGlyph => _stage.Story.Phase == TurnPhase.Generating ? "■" : "▶";

    partial void OnTextChanged(string value)
    {
        OnPropertyChanged(nameof(CanSend));
        OnPropertyChanged(nameof(SendGlyph));
        _stage.OnComposerTextChanged(value);
    }

    partial void OnNoteChanged(string value) => OnPropertyChanged(nameof(HasNote));

    partial void OnSelectedModelChanged(ModelQuickItem? value)
    {
        if (_suppressSelection || value is null) return;
        _stage.SwitchModelAsync(value.Id).SafeFireAndForget();
    }

    partial void OnSelectedEffortItemChanged(EffortItem? value)
    {
        if (_suppressSelection || value is null) return;
        _stage.SetEffortAsync(value.Value).SafeFireAndForget();
    }

    public void SetText(string? value) => Text = value ?? "";

    public void AppendText(string value) => Text = (Text ?? "") + value;

    public void Refresh()
    {
        OnPropertyChanged(nameof(Placeholder));
        OnPropertyChanged(nameof(IsDirectorModeBadgeVisible));
        OnPropertyChanged(nameof(IsFreeformModeBadgeVisible));
        RefreshModels();
        RaiseCan();
    }

    public void RefreshModels()
    {
        var settings = TavernAgent.Desktop.Services.AppServices.Settings;
        var primary = SettingsStore.ActivePrimaryInstance(settings.Providers, settings.Models);
        Models.Clear();
        foreach (var model in settings.Models)
            Models.Add(new ModelQuickItem(model.Id, model.Name, model.Id == primary?.Id));

        _suppressSelection = true;
        SelectedModel = Models.FirstOrDefault(m => m.IsPrimary);
        var effort = primary?.ReasoningEffort ?? "";
        SelectedEffortItem = Efforts.FirstOrDefault(e => e.Value == effort) ?? Efforts[0];
        _suppressSelection = false;
        OnPropertyChanged(nameof(HasModels));
    }

    public void RaiseCan()
    {
        OnPropertyChanged(nameof(CanSend));
        OnPropertyChanged(nameof(CanCancel));
        OnPropertyChanged(nameof(CanContinue));
        OnPropertyChanged(nameof(CanRetry));
        OnPropertyChanged(nameof(ShowSendButton));
        OnPropertyChanged(nameof(SendGlyph));
    }

    [RelayCommand]
    private async Task Send()
    {
        if (!CanSend) return;
        var value = Text;
        var note = IsNoteOpen && HasNote ? Note : null;
        Text = "";
        Note = "";
        IsNoteOpen = false;
        await _stage.SendAsync(value, null, note).ConfigureAwait(true);
    }

    [RelayCommand]
    private async Task Cancel() => await _stage.CancelAsync().ConfigureAwait(true);

    [RelayCommand]
    private async Task Continue() => await _stage.ContinueAsync().ConfigureAwait(true);

    [RelayCommand]
    private async Task Retry() => await _stage.RetryAsync().ConfigureAwait(true);

    [RelayCommand]
    private void ToggleNote() => IsNoteOpen = !IsNoteOpen;

    [RelayCommand]
    private void ApplyMacro(MacroPill? macro)
    {
        if (macro is null) return;
        Text = (Text ?? "") + macro.Text;
    }

    [RelayCommand]
    private async Task SwitchModel(string? modelId)
    {
        if (string.IsNullOrEmpty(modelId)) return;
        await _stage.SwitchModelAsync(modelId).ConfigureAwait(true);
    }

    [RelayCommand]
    private async Task SetEffort(string? effort)
    {
        await _stage.SetEffortAsync(effort ?? "").ConfigureAwait(true);
    }
}
