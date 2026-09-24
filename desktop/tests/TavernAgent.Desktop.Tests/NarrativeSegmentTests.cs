using System.Collections.Generic;
using System.Linq;
using TavernAgent.Core.Dtos;
using TavernAgent.Desktop.ViewModels.Stage;
using Xunit;

namespace TavernAgent.Desktop.Tests;

public class NarrativeSegmentTests
{
    private static IReadOnlyDictionary<string, string> Assets => new Dictionary<string, string>
    {
        ["smile"] = "/api/v1/cards/card_x/assets/smile.webp",
    };

    [Fact]
    public void Known_cmd_renders_as_image_segment()
    {
        var vm = new BlockViewModel(
            new TextBlock { Kind = "narration", Text = "前<img cmd=\"smile\">后" },
            "阿雾", "林舟", Assets);

        Assert.Equal(3, vm.Segments.Count);
        Assert.IsType<NarrativeTextSegment>(vm.Segments[0]);
        var image = Assert.IsType<NarrativeImageSegment>(vm.Segments[1]);
        Assert.Equal("smile", image.Alt);
        Assert.Equal("/api/v1/cards/card_x/assets/smile.webp", image.Url);
        Assert.IsType<NarrativeTextSegment>(vm.Segments[2]);
    }

    [Fact]
    public void Unknown_cmd_stays_literal_text()
    {
        var vm = new BlockViewModel(
            new TextBlock { Kind = "narration", Text = "<img cmd=\"missing\">" },
            "阿雾", "林舟", Assets);

        Assert.Single(vm.Segments);
        var text = Assert.IsType<NarrativeTextSegment>(vm.Segments[0]);
        Assert.Contains("missing", text.Markdown);
    }

    [Fact]
    public void Without_assets_text_is_untouched()
    {
        var vm = new BlockViewModel(
            new TextBlock { Kind = "dialogue", Text = "你好<img cmd=\"smile\">" },
            "阿雾", "林舟", null);

        Assert.Single(vm.Segments);
        Assert.IsType<NarrativeTextSegment>(vm.Segments[0]);
        Assert.Contains("你好", vm.Text);
    }

    [Fact]
    public void Bare_cmd_falls_back_to_numeric_variant()
    {
        var variants = new Dictionary<string, string>
        {
            ["anis_casual_excited1"] = "/a/1.webp",
            ["anis_casual_excited2"] = "/a/2.webp",
            ["anis_casual_excited3"] = "/a/3.webp",
        };
        var vm = new BlockViewModel(
            new TextBlock { Kind = "narration", Text = "<img cmd=\"anis_casual_excited\">" },
            "阿雾", "林舟", variants);

        var image = Assert.IsType<NarrativeImageSegment>(Assert.Single(vm.Segments));
        Assert.Contains(image.Url, new[] { "/a/1.webp", "/a/2.webp", "/a/3.webp" });
    }
}
