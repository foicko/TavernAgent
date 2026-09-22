using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Stage;

public sealed record MacroPill(string Label, string Text);

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
            new MacroPill("对白", "「」"),
            new MacroPill("动作", "**"),
            new MacroPill("潜台词", "（）"),
        };
        Refresh();
    }

    [ObservableProperty] private string _text = "";
    [ObservableProperty] private string _note = "";
    [ObservableProperty] private bool _isNoteOpen;
    [ObservableProperty] private ActionView? _selectedAction;
    [ObservableProperty] private ModelQuickItem? _selectedModel;
    [ObservableProperty] private EffortItem? _selectedEffortItem;

    private bool _suppressSelection;

    public ContextMetricsViewModel Context => _stage.Context;

    public DirectorViewModel Director => _stage.Director;

    [RelayCommand]
    private void OpenDirector() => _stage.OpenDirector();

    public ObservableCollection<ActionView> AvailableActions { get; } = new();
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
    public bool HasAction => SelectedAction is not null;
    public bool HasActions => AvailableActions.Count > 0;
    public bool HasModels => Models.Count > 0;
    public string ActionLabel => SelectedAction?.Label ?? "";

    public string Placeholder => _stage.Story.View is null
        ? "导入角色卡或打开一段会话即可开始推演…"
        : "输入你的行动或对白…（Enter 发送，Shift+Enter 换行）";

    public bool CanSend => !_stage.Busy && !_stage.IsHistoryView && !string.IsNullOrWhiteSpace(Text);
    public bool CanCancel => _stage.Story.Phase == TurnPhase.Generating;
    public bool CanContinue => _stage.Story.Phase == TurnPhase.Truncated;
    public bool CanRetry => _stage.Story.Phase == TurnPhase.Failed;
    public string SendGlyph => _stage.Story.Phase == TurnPhase.Generating ? "■" : "▶";

    partial void OnTextChanged(string value)
    {
        OnPropertyChanged(nameof(CanSend));
        OnPropertyChanged(nameof(SendGlyph));
        _stage.OnComposerTextChanged(value);
    }

    partial void OnNoteChanged(string value) => OnPropertyChanged(nameof(HasNote));

    partial void OnSelectedActionChanged(ActionView? value)
    {
        OnPropertyChanged(nameof(HasAction));
        OnPropertyChanged(nameof(ActionLabel));
    }

    partial void OnSelectedModelChanged(ModelQuickItem? value)
    {
        if (_suppressSelection || value is null) return;
        _ = _stage.SwitchModelAsync(value.Id);
    }

    partial void OnSelectedEffortItemChanged(EffortItem? value)
    {
        if (_suppressSelection || value is null) return;
        _ = _stage.SetEffortAsync(value.Value);
    }

    public void SetText(string? value) => Text = value ?? "";

    public void AppendText(string value) => Text = (Text ?? "") + value;

    public void Refresh()
    {
        AvailableActions.Clear();
        foreach (var action in _stage.Story.View?.Actions ?? new List<ActionView>())
            AvailableActions.Add(action);
        OnPropertyChanged(nameof(HasActions));
        OnPropertyChanged(nameof(Placeholder));
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
        OnPropertyChanged(nameof(SendGlyph));
    }

    [RelayCommand]
    private async Task Send()
    {
        if (!CanSend) return;
        var value = Text;
        var note = IsNoteOpen && HasNote ? Note : null;
        var actionRef = SelectedAction?.ActionId;
        Text = "";
        Note = "";
        IsNoteOpen = false;
        SelectedAction = null;
        await _stage.SendAsync(value, actionRef, note).ConfigureAwait(true);
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
    private void SelectAction(ActionView? action) => SelectedAction = action;

    [RelayCommand]
    private void ClearAction() => SelectedAction = null;

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
