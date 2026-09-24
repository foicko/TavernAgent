using System.Collections.Generic;
using TavernAgent.Core.CardMarkers;
using TavernAgent.Core.Dtos;
using Xunit;

namespace TavernAgent.Core.Tests;

public class EmotionAssetsTests
{
    private static CardAsset Asset(string name, string type) => new() { Name = name, Type = type, Url = "u/" + name };

    [Fact]
    public void Resolves_emotion_assets_by_name_keyword()
    {
        var assets = new List<CardAsset>
        {
            Asset("happy_1", "emotion"),
            Asset("angry_1", "emotion"),
            Asset("sad_1", "emotion"),
            Asset("neutral_1", "emotion"),
            Asset("room_bg", "x-risu-asset"),
        };

        Assert.Equal("u/happy_1", EmotionAssets.Resolve(assets, "smile"));
        Assert.Equal("u/angry_1", EmotionAssets.Resolve(assets, "alert"));
        Assert.Equal("u/sad_1", EmotionAssets.Resolve(assets, "whisper"));
        Assert.Equal("u/neutral_1", EmotionAssets.Resolve(assets, "calm"));
    }

    [Fact]
    public void Ignores_non_emotion_assets_and_returns_null_when_missing()
    {
        var assets = new List<CardAsset> { Asset("smile_scene", "x-risu-asset"), Asset("main", "icon") };
        Assert.Null(EmotionAssets.Resolve(assets, "smile"));
        Assert.Null(EmotionAssets.Resolve(assets, "unknown"));
        Assert.Null(EmotionAssets.Resolve(null, "smile"));
    }
}
