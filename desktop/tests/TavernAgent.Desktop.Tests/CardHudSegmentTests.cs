using System.Collections.Generic;
using System.Linq;
using TavernAgent.Desktop.ViewModels.Stage;
using Xunit;

namespace TavernAgent.Desktop.Tests;

public class CardHudSegmentTests
{
    [Fact]
    public void Fraction_values_become_meters()
    {
        var seg = new NarrativeHudSegment(false, "", new Dictionary<string, string>
        {
            ["理智"] = "82/100",
            ["体力"] = "40%",
        });

        var mind = seg.Values.Single(v => v.Label == "理智");
        Assert.True(mind.HasMeter);
        Assert.Equal("82/100", mind.Value);
        Assert.Equal(0.82, mind.Fraction, 3);

        var stamina = seg.Values.Single(v => v.Label == "体力");
        Assert.True(stamina.HasMeter);
        Assert.Equal(0.4, stamina.Fraction, 3);
    }

    [Fact]
    public void Plain_values_have_no_meter()
    {
        var seg = new NarrativeHudSegment(false, "", new Dictionary<string, string> { ["天气"] = "小雪" });
        var row = Assert.Single(seg.Values);
        Assert.False(row.HasMeter);
        Assert.Equal("小雪", row.Value);
    }

    [Fact]
    public void Scene_segment_exposes_title()
    {
        var seg = new NarrativeHudSegment(true, "教堂", null);
        Assert.True(seg.IsScene);
        Assert.Equal("教堂", seg.Title);
    }

    [Fact]
    public void Zero_denominator_is_not_a_meter()
    {
        var seg = new NarrativeHudSegment(false, "", new Dictionary<string, string> { ["x"] = "1/0" });
        Assert.False(Assert.Single(seg.Values).HasMeter);
    }
}
