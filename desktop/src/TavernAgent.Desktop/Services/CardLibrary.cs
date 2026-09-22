using System.Text.Json;
using TavernAgent.Core.Api;
using TavernAgent.Core.Dtos;

namespace TavernAgent.Desktop.Services;

/// <summary>Server-backed character card library helpers (no localStorage port).</summary>
public static class CardLibrary
{
    public static async Task<List<CardLibrarySummary>> ListAsync(TavernApiClient api, CancellationToken ct = default)
    {
        try
        {
            return await api.ListCardsAsync(ct).ConfigureAwait(false);
        }
        catch
        {
            return new List<CardLibrarySummary>();
        }
    }

    /// <summary>CardPreview carries no characterId; derive it from the embedded card JSON.</summary>
    public static string? ExtractCharacterId(CardPreview? card)
    {
        if (card is null) return null;
        if (!string.IsNullOrEmpty(card.CharacterJson))
        {
            try
            {
                using var doc = JsonDocument.Parse(card.CharacterJson);
                var root = doc.RootElement;
                if (root.TryGetProperty("characters", out var chars) &&
                    chars.ValueKind == JsonValueKind.Array && chars.GetArrayLength() > 0 &&
                    chars[0].TryGetProperty("characterId", out var cid) &&
                    cid.ValueKind == JsonValueKind.String)
                {
                    return cid.GetString();
                }
                if (root.TryGetProperty("cardId", out var pid) && pid.ValueKind == JsonValueKind.String)
                    return pid.GetString();
            }
            catch (JsonException)
            {
                // fall through to cardId
            }
        }
        return card.CardId;
    }
}
