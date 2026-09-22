using TavernAgent.Core.Api;
using TavernAgent.Core.Dtos;
using Xunit;

namespace TavernAgent.Core.Tests;

public class ResolveCharKeyTests
{
    private static WorldState Ws(Dictionary<string, CharacterInfo> characters) => new()
    {
        Relationships = new(),
        Items = new(),
        Promises = new(),
        Moods = new(),
        Characters = characters,
    };

    [Fact]
    public void Custom_Card_Session_With_Unmatched_Characters_Returns_Null()
    {
        var view = new SessionView
        {
            State = Ws(new Dictionary<string, CharacterInfo>
            {
                ["npc_other"] = new() { CharacterId = "npc_other", Name = "旅人" },
            }),
        };
        Assert.Null(Stores.StoryHelpers.ResolveCharKeyFromSession(view));
    }

    [Fact]
    public void Random_Custom_CharacterId_Is_Not_Mislabeled_As_Builtin()
    {
        var view = new SessionView
        {
            Title = "我的私设角色",
            State = Ws(new Dictionary<string, CharacterInfo>
            {
                ["npc_a3f9c2_k"] = new() { CharacterId = "npc_a3f9c2_k", Name = "阿雾" },
            }),
        };
        Assert.Null(Stores.StoryHelpers.ResolveCharKeyFromSession(view));
    }

    [Fact]
    public void Empty_View_And_Null_Return_Null()
    {
        Assert.Null(Stores.StoryHelpers.ResolveCharKeyFromSession(new SessionView()));
        Assert.Null(Stores.StoryHelpers.ResolveCharKeyFromSession(null));
    }

    [Fact]
    public void Unmatched_Title_Returns_Null()
    {
        Assert.Null(Stores.StoryHelpers.ResolveCharKeyFromSession(new SessionView { Title = "未知的故事" }));
    }

    [Fact]
    public void Custom_Key_Is_Never_A_Resolved_Preset()
    {
        var view = new SessionView
        {
            State = Ws(new Dictionary<string, CharacterInfo>
            {
                ["custom"] = new() { CharacterId = "custom", Name = "自定义角色" },
            }),
        };
        Assert.Null(Stores.StoryHelpers.ResolveCharKeyFromSession(view));
    }
}

public class StartupSessionTests
{
    private sealed record S(string SessionId) : Stores.IHasSessionId;

    [Fact]
    public void Prefers_Remembered_Story_Over_Match_And_Order()
    {
        var sessions = new[] { new S("s1"), new S("s2") };
        var picked = Stores.SessionMemory.ChooseStartupSession(sessions, "s1", s => s.SessionId == "s2");
        Assert.Equal("s1", picked?.SessionId);
    }

    [Fact]
    public void Falls_Back_To_Character_Match_Then_First()
    {
        var sessions = new[] { new S("s1"), new S("s2") };
        Assert.Equal("s2", Stores.SessionMemory.ChooseStartupSession(sessions, "gone", s => s.SessionId == "s2")?.SessionId);
        Assert.Equal("s1", Stores.SessionMemory.ChooseStartupSession(sessions, null, _ => false)?.SessionId);
        Assert.Null(Stores.SessionMemory.ChooseStartupSession(Array.Empty<S>(), "s1", _ => true));
    }
}

public class ModelLabelsTests
{
    [Fact]
    public void InputBudget_Matches_Backend_Formula()
    {
        Assert.Null(ModelLabelsInputBudgetHelper());
        Assert.Equal(8192 - 2048 - Math.Max(512, 8192 / 20), Stores.ModelLabels.InputBudget(8192, null));
        Assert.Equal(4096 - 512 - Math.Max(512, 4096 / 20), Stores.ModelLabels.InputBudget(4096, 512));
    }

    private static int? ModelLabelsInputBudgetHelper() => Stores.ModelLabels.InputBudget(null, null);

    [Fact]
    public void TokenLabel_Folds_Over_1k()
    {
        Assert.Equal("512", Stores.ModelLabels.TokenLabel(512));
        Assert.Equal("1k", Stores.ModelLabels.TokenLabel(1024));
    }

    [Fact]
    public void MaskedKeyHint_Never_Shows_Long_Plaintext()
    {
        Assert.Equal("已保存", Stores.ModelLabels.MaskedKeyHint(""));
        Assert.Equal("sk-1…abcd", Stores.ModelLabels.MaskedKeyHint("sk-1…abcd"));
        var longKey = new string('x', 40);
        var hint = Stores.ModelLabels.MaskedKeyHint(longKey);
        Assert.DoesNotContain(longKey, hint);
        Assert.StartsWith("xxxx", hint);
    }
}
