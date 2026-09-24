using System.Collections.Generic;
using TavernAgent.Desktop.ViewModels.Stage;
using Xunit;

namespace TavernAgent.Desktop.Tests;

public class NarrativeTextMacroTests
{
    [Fact]
    public void Expands_identity_macros()
    {
        Assert.Equal("阿尔 看向 旅人", NarrativeText.ReplaceMacros("{{char}} 看向 {{user}}", "阿尔", "旅人"));
    }

    [Fact]
    public void Resolves_getvar_from_variables()
    {
        var vars = new Dictionary<string, string> { ["lang"] = "KO", ["scene"] = "3" };
        var outText = NarrativeText.ReplaceMacros("语言：{{getvar::lang}}，场景 {{ getvar::scene }}", "阿尔", null, vars);
        Assert.Equal("语言：KO，场景 3", outText);
    }

    [Fact]
    public void Missing_getvar_is_empty()
    {
        Assert.Equal("!", NarrativeText.ReplaceMacros("{{getvar::missing}}!", "阿尔", null, new Dictionary<string, string>()));
        Assert.Equal("!", NarrativeText.ReplaceMacros("{{getvar::missing}}!", "阿尔", null));
    }

    [Fact]
    public void CleanShortName_preserves_english_word_boundaries_and_chinese_titles()
    {
        Assert.Equal("The Magical Revolution", NarrativeText.CleanShortName("The Magical Revolution of Reincarnated Princess and Genius Young Lady"));
        Assert.Equal("转生王女与天才千金的魔法革命", NarrativeText.CleanShortName("转生王女与天才千金的魔法革命"));
        Assert.Equal("艾妮丝菲亚", NarrativeText.CleanShortName("艾妮丝菲亚 - 帕雷提亚王国第一王女"));
        Assert.Equal("自定义", NarrativeText.CleanShortName("   "));
    }

    [Fact]
    public void TrimDialogue_strips_nested_chinese_and_western_quotes()
    {
        Assert.Equal("噗——？！", NarrativeText.TrimDialogue("“噗——？！”"));
        Assert.Equal("噗——？！", NarrativeText.TrimDialogue("「“噗——？！”」"));
        Assert.Equal("你好", NarrativeText.TrimDialogue("『“你好”』"));
    }
}
