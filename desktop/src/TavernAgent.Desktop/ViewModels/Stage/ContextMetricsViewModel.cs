using System.Globalization;
using System.Text.RegularExpressions;
using CommunityToolkit.Mvvm.ComponentModel;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Stage;

/// <summary>Context-window occupancy + prefix-cache estimate (web: tokenEstimator.calculateContextMetrics).</summary>
public sealed partial class ContextMetricsViewModel : ObservableObject
{
    private static readonly Regex Cjk = new(@"[\u4e00-\u9fa5\u3040-\u30ff\uac00-\ud7af]", RegexOptions.Compiled);

    [ObservableProperty] private double _percentage;
    [ObservableProperty] private string _usedFormatted = "0";
    [ObservableProperty] private string _limitFormatted = "64K";
    [ObservableProperty] private double _cacheHitRate;
    [ObservableProperty] private string _modelName = "";
    [ObservableProperty] private int _systemTokens;
    [ObservableProperty] private int _memoryTokens;
    [ObservableProperty] private int _historyTokens;
    [ObservableProperty] private int _draftTokens;
    [ObservableProperty] private int _turnCount;

    public string PercentageText => $"{Percentage.ToString("0.#", CultureInfo.InvariantCulture)}%";

    public bool IsWarn => Percentage >= 85;

    public double SweepAngle => Math.Clamp(Percentage, 0, 100) / 100.0 * 360.0;

    public string SystemLabel => $"{SystemTokens:N0} tk";
    public string MemoryLabel => $"{MemoryTokens:N0} tk";
    public string HistoryLabel => $"{HistoryTokens:N0} tk";
    public string DraftLabel => $"{DraftTokens:N0} tk";
    public string CacheLabel => $"{CacheHitRate.ToString("0.#", CultureInfo.InvariantCulture)}%";

    public void Recalculate(StoryStore story, ModelInstance? primary, string inputText)
    {
        var view = story.View;

        // System persona / world rules (no full card available on desktop → use character metadata).
        var systemText = "";
        if (view is not null)
        {
            var character = view.State?.Characters.Values.FirstOrDefault(c => c.Participant == true) ?? view.State?.Characters.Values.FirstOrDefault(c => c.CharacterId != "player");
            systemText += $"{character?.Name} \n{character?.Description}\n{character?.Personality}\n{character?.Scenario}\n";
        }
        SystemTokens = Estimate(systemText) + (view is not null ? 200 : 0);

        // Memory bank + worldbook + active summary + revealed secrets.
        var memoryText = "";
        if (view?.ActiveSummary?.Text is { Length: > 0 } summary) memoryText += summary + "\n";
        if (view?.Secrets is not null)
        {
            foreach (var s in view.Secrets)
                if (s.Revealed && !string.IsNullOrEmpty(s.Content)) memoryText += $"{s.Title}: {s.Content}\n";
        }
        MemoryTokens = Estimate(memoryText);

        // Conversation history.
        var historyText = "";
        var turnCount = 0;
        foreach (var msg in story.Messages)
        {
            if (msg.Role == "user") { turnCount++; historyText += (msg.InputText ?? "") + "\n"; }
            else
            {
                if (msg.Role == "assistant") turnCount++;
                historyText += string.Join("\n", msg.Blocks.Select(b => b.Text)) + "\n";
                if (!string.IsNullOrEmpty(msg.Thinking)) historyText += msg.Thinking + "\n";
            }
        }
        HistoryTokens = Estimate(historyText);
        TurnCount = turnCount;

        DraftTokens = Estimate(inputText);

        var total = Math.Max(0, SystemTokens + MemoryTokens + HistoryTokens + DraftTokens);
        var window = primary?.ContextWindow is > 0 ? primary.ContextWindow.Value : 65536;
        Percentage = Math.Min(100, Math.Round((double)total / window * 1000) / 10);
        UsedFormatted = FormatTokenCount(total);
        LimitFormatted = FormatTokenCount(window);
        ModelName = primary?.Model ?? "";

        var staticPrefix = SystemTokens + MemoryTokens;
        var cachedHistory = HistoryTokens > 500 ? HistoryTokens * 0.88 : HistoryTokens * 0.7;
        var totalCached = staticPrefix + cachedHistory;
        CacheHitRate = total > 80
            ? Math.Min(96.4, Math.Max(45.0, Math.Round(totalCached / total * 1000) / 10))
            : 0;

        OnPropertyChanged(nameof(PercentageText));
        OnPropertyChanged(nameof(IsWarn));
        OnPropertyChanged(nameof(SweepAngle));
        OnPropertyChanged(nameof(SystemLabel));
        OnPropertyChanged(nameof(MemoryLabel));
        OnPropertyChanged(nameof(HistoryLabel));
        OnPropertyChanged(nameof(DraftLabel));
        OnPropertyChanged(nameof(CacheLabel));
    }

    public static int Estimate(string? text)
    {
        if (string.IsNullOrEmpty(text)) return 0;
        var cjk = Cjk.Matches(text).Count;
        var nonCjk = text.Length - cjk;
        return (int)Math.Ceiling(cjk + nonCjk * 0.28);
    }

    public static string FormatTokenCount(int tokens)
    {
        if (tokens >= 1_000_000) return $"{tokens / 1_000_000.0:0.#}M";
        if (tokens >= 1_000) return $"{tokens / 1_000.0:0.#}K";
        return tokens.ToString(CultureInfo.InvariantCulture);
    }
}
