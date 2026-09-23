using System.ComponentModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Stage;

/// <summary>
/// One committed narrative turn (web: HistoricalStory turn node). Opening nodes use Role == "opening".
/// </summary>
public sealed partial class MessageViewModel : ObservableObject
{
    private readonly StageViewModel _stage;

    public MessageViewModel(
        StoryMessage msg,
        int index,
        bool isLast,
        StageViewModel stage,
        string charName,
        string userName,
        IReadOnlyDictionary<string, string> actionLabels)
    {
        _stage = stage;
        Id = msg.Id;
        ParentId = msg.ParentId;
        Role = msg.Role;
        Index = index;
        IsLast = isLast;
        IsOpening = msg.Role == "opening";
        IsAssistant = msg.Role == "assistant";
        TurnNumber = msg.TurnNumber;
        Draft = msg.Draft;
        TurnLabel = $"回合 #{msg.TurnNumber ?? index}";

        Blocks = msg.Blocks.Select(b => new BlockViewModel(b, charName, userName)).ToList();
        Options = msg.Options.Select(o => new OptionViewModel(o)).ToList();
        InputText = string.IsNullOrEmpty(msg.InputText) ? null : NarrativeText.ReplaceMacros(msg.InputText, charName, userName);
        InputNote = string.IsNullOrWhiteSpace(msg.InputNote) ? null : msg.InputNote;
        Thinking = string.IsNullOrEmpty(msg.Thinking) ? null : msg.Thinking;
        SuppressedOptions = msg.SuppressedOptions ?? 0;

        Checks = (msg.Checks ?? new List<CheckResult>())
            .Select(c => new CheckEvidenceViewModel(c, actionLabels.TryGetValue(c.ActionId, out var l) ? l : null))
            .ToList();
        Changes = (msg.Changes ?? new List<StateChange>()).Select(c => new ChangeEvidenceViewModel(c)).ToList();
        Memories = (msg.InjectedMemories ?? new List<MemoryRef>()).Select(m => new MemoryEvidenceViewModel(m)).ToList();

        HasInputText = InputText is not null;
        HasThinking = Thinking is not null;
        ShowToolbar = !Draft;

        var detailCounts = new List<string>();
        if (Checks.Count > 0) detailCounts.Add($"{Checks.Count} 次检定");
        if (Memories.Count > 0) detailCounts.Add($"{Memories.Count} 条记忆");
        EvidenceCounts = string.Join(" · ", detailCounts);
        HasEvidenceDetail = Checks.Count > 0 || Memories.Count > 0;
        HasEvidenceLine = InputNote is not null || Changes.Count > 0 || Checks.Count > 0 || SuppressedOptions > 0;
        HasEvidence = HasEvidenceLine || HasEvidenceDetail;

        _stage.PropertyChanged += OnStageChanged;
        Illustration = new TurnIllustrationViewModel(stage, Id, PlainText);
        Tts = new TtsButtonViewModel(stage, PlainText, ExtractDialogue());
    }

    public string Id { get; }
    public string? ParentId { get; }
    public string Role { get; }
    public int Index { get; }
    public bool IsLast { get; }
    public bool IsOpening { get; }
    public bool IsAssistant { get; }
    public int? TurnNumber { get; }
    public bool Draft { get; }
    public string TurnLabel { get; }

    public IReadOnlyList<BlockViewModel> Blocks { get; }
    public IReadOnlyList<OptionViewModel> Options { get; }
    public string? InputText { get; }
    public string? InputNote { get; }
    public string? Thinking { get; }
    public int SuppressedOptions { get; }

    public IReadOnlyList<CheckEvidenceViewModel> Checks { get; }
    public IReadOnlyList<ChangeEvidenceViewModel> Changes { get; }
    public IReadOnlyList<MemoryEvidenceViewModel> Memories { get; }

    public bool HasInputText { get; }
    public bool HasInputNote => InputNote is not null;
    public bool HasThinking { get; }
    public bool ShowToolbar { get; }
    public bool HasEvidence { get; }
    public bool HasEvidenceLine { get; }
    public bool HasEvidenceDetail { get; }
    public string EvidenceCounts { get; }

    public bool HasChecks => Checks.Count > 0;
    public bool HasMemories => Memories.Count > 0;
    public bool HasSuppressed => SuppressedOptions > 0;
    public bool HasOptions => Options.Count > 0;

    public string PlainText => string.Join("\n", Blocks.Select(b => b.Text));

    public TurnIllustrationViewModel Illustration { get; }

    public TtsButtonViewModel Tts { get; }

    private string ExtractDialogue()
    {
        var dialogue = Blocks.Where(b => b.IsDialogue).Select(b => b.Text.Trim()).Where(t => t.Length > 0).ToList();
        if (dialogue.Count > 0) return string.Join("\n", dialogue);

        // Fallback: pull quoted spans out of the plain text.
        var matches = System.Text.RegularExpressions.Regex.Matches(PlainText, @"[「“『""]([^」”』""]+)[」”』""]");
        return string.Join("\n", matches.Select(m => m.Groups[1].Value.Trim()).Where(t => t.Length > 0));
    }

    [ObservableProperty]
    private bool _isThinkingExpanded;

    [ObservableProperty]
    private bool _isEvidenceExpanded;

    [ObservableProperty]
    private bool _isEditingText;

    [ObservableProperty]
    private string _editText = "";

    [ObservableProperty]
    private bool _isEditingInput;

    [ObservableProperty]
    private string _editInputText = "";

    [ObservableProperty]
    private bool _isRegenOpen;

    [ObservableProperty]
    private string _regenGuidance = "";

    public bool CanAct => !_stage.Busy && !_stage.IsHistoryView;

    private IReadOnlyList<PlotNode> CandidateGroup()
    {
        var groups = _stage.Story.View?.CandidateGroups;
        if (groups is null || string.IsNullOrEmpty(ParentId) || !groups.TryGetValue(ParentId, out var g))
            return Array.Empty<PlotNode>();
        return g;
    }

    private int CandidateIndex()
    {
        var g = CandidateGroup();
        for (var i = 0; i < g.Count; i++) if (g[i].NodeId == Id) return i;
        return -1;
    }

    public bool HasCandidates
    {
        get
        {
            var g = CandidateGroup();
            return g.Count > 1 && CandidateIndex() >= 0;
        }
    }

    public string CandidateLabel
    {
        get
        {
            var idx = CandidateIndex();
            var count = CandidateGroup().Count;
            return idx < 0 ? "" : $"版本 {idx + 1} / {count}";
        }
    }

    [RelayCommand]
    private async Task PrevCandidate()
    {
        var g = CandidateGroup();
        var idx = CandidateIndex();
        if (idx > 0) await _stage.ViewAtNodeAsync(g[idx - 1].NodeId).ConfigureAwait(true);
    }

    [RelayCommand]
    private async Task NextCandidate()
    {
        var g = CandidateGroup();
        var idx = CandidateIndex();
        if (idx >= 0 && idx < g.Count - 1) await _stage.ViewAtNodeAsync(g[idx + 1].NodeId).ConfigureAwait(true);
    }

    public bool ShowDecisionDeck => IsLast && IsAssistant && HasOptions && _stage.IsIdle && _stage.IsStudioMode;

    public string EvidenceToggleLabel => IsEvidenceExpanded ? "收起依据" : $"展开依据（{EvidenceCounts}）";

    public string ThinkingToggleLabel => IsThinkingExpanded ? "收起" : "展开";

    [RelayCommand]
    private void ToggleThinking() => IsThinkingExpanded = !IsThinkingExpanded;

    [RelayCommand]
    private void ToggleEvidence()
    {
        IsEvidenceExpanded = !IsEvidenceExpanded;
        OnPropertyChanged(nameof(EvidenceToggleLabel));
    }

    [RelayCommand]
    private void ChooseOption(OptionViewModel? option)
    {
        if (option is not null) _stage.ChooseOptionAsync(option).SafeFireAndForget();
    }

    public void NotifyCopied() => _stage.Ui.NotifyQuiet("已复制文学正文到剪贴板");

    [RelayCommand]
    private void Quote() => _stage.InjectQuote(PlainText);

    [RelayCommand]
    private void DrawIllustration() => _stage.OpenIllustration(Id, PlainText);

    [RelayCommand]
    private async Task Regenerate() => await _stage.RegenerateAsync(this, null).ConfigureAwait(true);

    [RelayCommand]
    private async Task Recheck() => await _stage.RegenerateAsync(this, null, recheck: true).ConfigureAwait(true);

    [RelayCommand]
    private void ToggleRegen()
    {
        IsRegenOpen = !IsRegenOpen;
        if (IsRegenOpen) { IsEditingText = false; IsEditingInput = false; }
    }

    [RelayCommand]
    private async Task SubmitRegen()
    {
        var guidance = string.IsNullOrWhiteSpace(RegenGuidance) ? null : RegenGuidance.Trim();
        IsRegenOpen = false;
        RegenGuidance = "";
        await _stage.RegenerateAsync(this, guidance).ConfigureAwait(true);
    }

    [RelayCommand]
    private void ToggleEditText()
    {
        if (!IsEditingText) EditText = string.Join("\n", Blocks.Select(b => b.Text));
        IsEditingText = !IsEditingText;
        if (IsEditingText) { IsEditingInput = false; IsRegenOpen = false; }
    }

    [RelayCommand]
    private async Task SubmitEditText()
    {
        var blocks = EditText.Split('\n')
            .Where(l => l.Length > 0)
            .Select(l => new TextBlock { Kind = "narration", Text = l })
            .ToList();
        IsEditingText = false;
        await _stage.EditTextAsync(this, blocks).ConfigureAwait(true);
    }

    [RelayCommand]
    private void ToggleEditInput()
    {
        if (!IsEditingInput) EditInputText = InputText ?? "";
        IsEditingInput = !IsEditingInput;
        if (IsEditingInput) { IsEditingText = false; IsRegenOpen = false; }
    }

    [RelayCommand]
    private async Task SubmitEditInput()
    {
        var text = EditInputText.Trim();
        IsEditingInput = false;
        await _stage.EditInputAsync(this, text).ConfigureAwait(true);
    }

    [RelayCommand]
    private async Task Fork() => await _stage.ForkAsync(this).ConfigureAwait(true);

    private void OnStageChanged(object? sender, PropertyChangedEventArgs e)
    {
        OnPropertyChanged(nameof(CanAct));
        OnPropertyChanged(nameof(ShowDecisionDeck));
        OnPropertyChanged(nameof(HasCandidates));
        OnPropertyChanged(nameof(CandidateLabel));
    }

    public void Detach()
    {
        _stage.PropertyChanged -= OnStageChanged;
        Tts.Detach();
    }
}
