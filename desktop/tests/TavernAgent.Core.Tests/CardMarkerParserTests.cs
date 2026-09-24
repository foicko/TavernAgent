using TavernAgent.Core.CardMarkers;
using Xunit;

namespace TavernAgent.Core.Tests;

public class CardMarkerParserTests
{
    private static readonly Dictionary<string, string> Assets = new()
    {
        ["smile"] = "/api/v1/cards/card_x/assets/smile.webp",
    };

    [Fact]
    public void Strips_state_markers_and_keeps_text()
    {
        var segs = CardMarkerParser.Parse("雪停了。[SCENE:教堂][HUD:理智=80|体力=55]", Assets);
        Assert.Equal(
            new[] { CardMarkerKind.Text, CardMarkerKind.Scene, CardMarkerKind.Hud },
            segs.Select(s => s.Kind));
        Assert.Equal("雪停了。", segs[0].Text);
        Assert.Equal("教堂", segs[1].Title);
        Assert.Equal("80", segs[2].Values!["理智"]);
    }

    [Fact]
    public void Parses_both_image_marker_forms()
    {
        var segs = CardMarkerParser.Parse("<img cmd=\"smile\"> [IMG:smile]", Assets);
        Assert.Equal(2, segs.Count(s => s.Kind == CardMarkerKind.Image));
    }

    [Fact]
    public void Var_markers_are_not_displayed()
    {
        var segs = CardMarkerParser.Parse("[VAR:lang=KO]正文", Assets);
        Assert.Single(segs);
        Assert.Equal(CardMarkerKind.Text, segs[0].Kind);
        Assert.Equal("正文", segs[0].Text);
    }

    [Fact]
    public void Escaped_bracket_is_literal()
    {
        var segs = CardMarkerParser.Parse("写作 [[HUD:x]", Assets);
        Assert.Single(segs);
        Assert.Equal("写作 [HUD:x]", segs[0].Text);
    }

    [Fact]
    public void Unresolved_image_marker_stays_literal()
    {
        var segs = CardMarkerParser.Parse("<img cmd=\"unknown\">", Assets);
        Assert.Single(segs);
        Assert.Equal(CardMarkerKind.Text, segs[0].Kind);
        Assert.Equal("<img cmd=\"unknown\">", segs[0].Text);
    }

    [Fact]
    public void Null_text_returns_empty()
    {
        Assert.Empty(CardMarkerParser.Parse(null, Assets));
    }
}
