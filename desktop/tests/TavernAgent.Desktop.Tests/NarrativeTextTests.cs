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
}
