using System.Text.Json;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Core.Stores;

public sealed class CharacterPreset
{
    public string Key { get; init; } = "custom";
    public string Name { get; init; } = "未选择角色";
    public string ShortName { get; init; } = "未选择";
    public string Role { get; init; } = "请在左侧栏导入角色卡";
    public string StagePill { get; init; } = "暂无角色";
    public string StateBadge { get; init; } = "状态：待定";
    public string AllianceBadge { get; init; } = "关系：尚未建立";
    public string VoiceBadge { get; init; } = "叙事：待定";
    public string Avatar { get; init; } = "";
    public string FullImg { get; init; } = "";
    public string ModalDesc { get; init; } = "";
    public string InputPlaceholder { get; init; } = "输入你的行动或对白…（Enter 发送，Shift+Enter 换行）";
    public string Prologue { get; init; } = "";
}

/// <summary>
/// Neutral custom-skin only (parity with characterPresets.ts: no bundled third-party cards).
/// </summary>
public static class CharacterPresets
{
    public const string DefaultPresetKey = "custom";
    public const string BuiltinCharacterName = "";

    public static readonly IReadOnlyList<string> SelectablePresetKeys = Array.Empty<string>();

    public static readonly CharacterPreset Neutral = new()
    {
        Key = "custom",
        Name = "未选择角色",
        ShortName = "未选择",
        Role = "请在左侧栏导入角色卡",
        StagePill = "暂无角色",
        StateBadge = "状态：待定",
        AllianceBadge = "关系：尚未建立",
        VoiceBadge = "叙事：待定",
        Avatar = "/assets/original/custom.svg",
        FullImg = "/assets/original/custom.svg",
        ModalDesc = "尚未载入角色卡。请点击左侧栏的「+ 导入 / 创建角色卡」，上传 PNG 或 JSON 角色卡开启剧情。",
        InputPlaceholder = "输入你的行动或对白…（Enter 发送，Shift+Enter 换行）",
        Prologue = "",
    };

    public static readonly IReadOnlyDictionary<string, CharacterPreset> Presets =
        new Dictionary<string, CharacterPreset>(StringComparer.Ordinal) { ["custom"] = Neutral };

    public static string GetPresetCardJson(string charKey)
    {
        var c = Presets.TryGetValue(charKey, out var p) ? p : Neutral;
        var payload = new Dictionary<string, object?>
        {
            ["schemaVersion"] = 2,
            ["cardId"] = $"tavernagent_original_{c.Key}_v1",
            ["name"] = c.ShortName,
            ["description"] = c.ModalDesc,
            ["first_mes"] = c.Prologue,
            ["characters"] = new[]
            {
                new Dictionary<string, object?>
                {
                    ["characterId"] = $"npc_{c.Key}",
                    ["name"] = c.ShortName,
                    ["description"] = c.ModalDesc,
                    ["participant"] = true,
                    ["avatar"] = c.Avatar,
                },
            },
        };
        return JsonSerializer.Serialize(payload, JsonDefaults.Indented);
    }

    public static string PresetIdentity(string key)
    {
        using var doc = JsonDocument.Parse(GetPresetCardJson(key));
        return doc.RootElement.GetProperty("cardId").GetString() ?? "";
    }
}
