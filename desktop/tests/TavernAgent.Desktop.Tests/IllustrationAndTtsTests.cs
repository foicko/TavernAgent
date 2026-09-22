using System.Linq;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.Services;
using Xunit;

namespace TavernAgent.Desktop.Tests;

public class IllustrationAndTtsTests
{
    [Fact]
    public void Prompt_Extractor_Cleans_And_Wraps()
    {
        var extracted = ImagePromptExtractor.Extract(
            new[] { "雨夜里，**她**推开木门（神情疲惫）。", "*灯火*摇曳。" },
            null, "anime", "阿雾");

        Assert.DoesNotContain("（", extracted.Prompt);
        Assert.DoesNotContain("*", extracted.Prompt);
        Assert.StartsWith("Character: 阿雾, ", extracted.Prompt);
        Assert.Contains("anime visual novel CG", extracted.Prompt);
        Assert.Contains("low quality", extracted.NegativePrompt);
    }

    [Fact]
    public void Prompt_Extractor_Truncates_Excerpt()
    {
        var longText = string.Concat(Enumerable.Repeat("夜色低垂灯火摇曳。", 40));
        var extracted = ImagePromptExtractor.Extract(null, longText, "cinematic", null);
        Assert.True(extracted.Excerpt.Length <= 180);
    }

    [Fact]
    public void Illustration_Store_Attach_Get_Remove()
    {
        var store = new IllustrationStore();
        var id = "test-turn-" + Guid.NewGuid().ToString("N");
        var result = new GeneratedImageResult { Id = "img1", Url = "/api/v1/images/img1.png" };

        Assert.Null(store.Get(id));
        var changed = 0;
        store.Changed += (_, _) => changed++;
        store.Attach(id, result);
        Assert.NotNull(store.Get(id));
        Assert.Equal(1, changed);
        store.Remove(id);
        Assert.Null(store.Get(id));
        Assert.Equal(2, changed);
    }

    [Fact]
    public void Tts_Director_Detects_MiMo_And_Enriches()
    {
        Assert.True(TtsDirector.IsMiMoEngine("mimo", null, null));
        Assert.True(TtsDirector.IsMiMoEngine("openai", "mimo-v2.5-tts", null));
        Assert.True(TtsDirector.IsMiMoEngine("openai", null, "https://api.xiaomimimo.com/v1"));
        Assert.False(TtsDirector.IsMiMoEngine("edge", "tts-1", "http://127.0.0.1:9880/v1"));

        var enriched = TtsDirector.Enrich("你好呀。", "愤懑");
        Assert.Contains("(愤懑)", enriched);
        Assert.Contains("你好呀。", enriched);

        var instruction = TtsDirector.BuildInstruction("阿雾", "雨夜客栈", "不安");
        Assert.Contains("【角色】阿雾", instruction);
        Assert.Contains("【场景】雨夜客栈", instruction);
    }
}
