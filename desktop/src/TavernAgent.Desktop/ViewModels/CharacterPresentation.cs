using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.ViewModels.Stage;

namespace TavernAgent.Desktop.ViewModels;

/// <summary>Resolved display identity for the active character (web: characterPresentation).</summary>
public sealed class CharacterPresentation
{
    public string Name { get; init; } = CharacterPresets.Neutral.Name;
    public string ShortName { get; init; } = CharacterPresets.Neutral.ShortName;
    public string Role { get; init; } = CharacterPresets.Neutral.Role;
    public string StagePill { get; init; } = CharacterPresets.Neutral.StagePill;
    public string StateBadge { get; init; } = CharacterPresets.Neutral.StateBadge;
    public string AllianceBadge { get; init; } = CharacterPresets.Neutral.AllianceBadge;
    public string VoiceBadge { get; init; } = CharacterPresets.Neutral.VoiceBadge;
    public string ModalDesc { get; init; } = CharacterPresets.Neutral.ModalDesc;
    public string? AvatarSource { get; init; }
    public string PlayerName { get; init; } = NarrativeText.DefaultPlayerName;
    public IReadOnlyList<string> Tags { get; init; } = Array.Empty<string>();

    public string Description { get; init; } = "";
    public string Personality { get; init; } = "";
    public string Scenario { get; init; } = "";
    public string MesExample { get; init; } = "";
    public string SystemPrompt { get; init; } = "";
    public string PostHistoryInstructions { get; init; } = "";
    public string CreatorNotes { get; init; } = "";
    public string Creator { get; init; } = "";
    public string CharacterVersion { get; init; } = "";

    public bool HasAvatar => !string.IsNullOrEmpty(AvatarSource);
    public bool HasTags => Tags.Count > 0;
    public string Monogram => string.IsNullOrEmpty(ShortName) ? "?" : ShortName[..1];

    public string PledgeTitle => "尚未立下契约";
    public bool IsActive => StagePill == "当前主角" || StagePill == "已载入";
    public bool ShowSubtitle => !string.IsNullOrWhiteSpace(Name) && !string.Equals(Name.Trim(), ShortName.Trim(), StringComparison.Ordinal);

    private static readonly HashSet<string> IgnoredMetaTags = new(StringComparer.OrdinalIgnoreCase)
    {
        "nsfw", "sfw", "english", "chinese", "female", "male", "human",
        "anime", "fictional character", "multiple greetings", "feet", "original character", "oc"
    };

    public static IReadOnlyList<string> SanitizeDisplayTags(IEnumerable<string>? rawTags, int maxCount = 4)
    {
        if (rawTags is null) return Array.Empty<string>();

        var result = new List<string>();
        foreach (var tag in rawTags)
        {
            if (string.IsNullOrWhiteSpace(tag)) continue;
            var trimmed = tag.Trim();
            if (trimmed.StartsWith('#')) trimmed = trimmed[1..].Trim();
            if (string.IsNullOrWhiteSpace(trimmed)) continue;

            if (trimmed.Length > 18) continue;
            if (IgnoredMetaTags.Contains(trimmed)) continue;

            result.Add(trimmed);
            if (result.Count >= maxCount) break;
        }

        return result;
    }

    public static CharacterPresentation Resolve(string? charKey, SessionView? view)
    {
        var preset = CharacterPresets.Neutral;
        CharacterInfo? character = null;
        if (view?.State?.Characters is { Count: > 0 } chars)
            character = chars.Values.FirstOrDefault(c => c.Participant == true)
                        ?? chars.Values.FirstOrDefault(c => c.CharacterId != "player");

        var rawName = !string.IsNullOrWhiteSpace(character?.Name) ? character!.Name!
            : (!string.IsNullOrWhiteSpace(view?.Title) ? view!.Title : preset.Name);
        var shortName = NarrativeText.CleanShortName(
            !string.IsNullOrWhiteSpace(character?.Nickname) ? character!.Nickname! : rawName);
        var player = view?.State?.Characters.TryGetValue("player", out var p) == true && !string.IsNullOrWhiteSpace(p?.Name)
            ? p!.Name! : NarrativeText.DefaultPlayerName;

        var avatar = character?.Avatar;
        var avatarUsable = !string.IsNullOrEmpty(avatar) &&
            (avatar.StartsWith("data:", StringComparison.OrdinalIgnoreCase)
             || avatar.StartsWith("http", StringComparison.OrdinalIgnoreCase)
             || File.Exists(avatar));

        var hasActiveStory = view is not null;
        var hasActiveChar = character is not null;

        var stagePill = hasActiveChar ? "当前主角" : (hasActiveStory ? "已载入" : preset.StagePill);
        var stateBadge = hasActiveStory ? "状态：持续同步" : preset.StateBadge;
        var allianceBadge = hasActiveStory ? "关系：主线羁绊" : preset.AllianceBadge;
        var voiceBadge = hasActiveStory ? "叙事：沉浸演绎" : preset.VoiceBadge;

        return new CharacterPresentation
        {
            Name = rawName,
            ShortName = shortName,
            Role = NarrativeText.FirstLine(character?.Description) ?? preset.Role,
            StagePill = stagePill,
            StateBadge = stateBadge,
            AllianceBadge = allianceBadge,
            VoiceBadge = voiceBadge,
            ModalDesc = string.IsNullOrWhiteSpace(character?.Description) ? preset.ModalDesc : character!.Description!,
            AvatarSource = avatarUsable ? avatar : null,
            PlayerName = player,
            Tags = SanitizeDisplayTags(character?.Tags),
            Description = character?.Description ?? "",
            Personality = character?.Personality ?? "",
            Scenario = character?.Scenario ?? "",
            MesExample = character?.MesExample ?? "",
            SystemPrompt = character?.SystemPrompt ?? "",
            PostHistoryInstructions = character?.PostHistoryInstructions ?? "",
            CreatorNotes = character?.CreatorNotes ?? "",
            Creator = character?.Creator ?? "",
            CharacterVersion = character?.CharacterVersion ?? "",
        };
    }
}
