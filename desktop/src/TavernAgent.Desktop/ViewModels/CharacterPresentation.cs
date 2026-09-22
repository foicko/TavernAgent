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

        return new CharacterPresentation
        {
            Name = rawName,
            ShortName = shortName,
            Role = NarrativeText.FirstLine(character?.Description) ?? preset.Role,
            StagePill = preset.StagePill,
            StateBadge = preset.StateBadge,
            AllianceBadge = preset.AllianceBadge,
            VoiceBadge = preset.VoiceBadge,
            ModalDesc = string.IsNullOrWhiteSpace(character?.Description) ? preset.ModalDesc : character!.Description!,
            AvatarSource = avatarUsable ? avatar : null,
            PlayerName = player,
            Tags = character?.Tags ?? new List<string>(),
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
