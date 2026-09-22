using Avalonia;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using Avalonia.Threading;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.ViewModels;
using TavernAgent.Desktop.ViewModels.Stage;
using TavernAgent.Desktop.Views.Stage;
using Xunit;
using TextBlock = TavernAgent.Core.Dtos.TextBlock;

namespace TavernAgent.Desktop.Tests;

public class StageViewModelTests
{
    [AvaloniaFact]
    public void Stage_Constructs_Empty_Without_Story()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        Assert.False(ws.Stage.HasStory);
        Assert.False(ws.Stage.ShowPrologue);
        Assert.Empty(ws.Stage.Messages);
        Assert.Equal(1, ws.Stage.Composer.Efforts.Count > 0 ? 1 : 0);
        ws.Dispose();
    }

    [AvaloniaFact]
    public void Message_Blocks_Render_Macros_And_Kind_Wrappers()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        var msg = new StoryMessage
        {
            Id = "n1",
            Role = "assistant",
            Blocks = new List<TextBlock>
            {
                new() { Kind = "narration", Text = "**{{char}}** 笑了。" },
                new() { Kind = "dialogue", Text = "「你好」" },
                new() { Kind = "inner_monologue", Text = "（{{user}}在想什么）" },
            },
            InputText = "推门而入",
        };

        var vm = new MessageViewModel(msg, 0, isLast: true, ws.Stage, "阿雾", "旅人", new Dictionary<string, string>());

        Assert.True(vm.Blocks[0].IsNarration);
        Assert.Equal("**阿雾** 笑了。", vm.Blocks[0].Text);
        Assert.True(vm.Blocks[1].IsDialogue);
        Assert.Equal("「你好」", vm.Blocks[1].Text);
        Assert.True(vm.Blocks[2].IsMonologue);
        Assert.Equal("（旅人在想什么）", vm.Blocks[2].Text);
        Assert.True(vm.HasInputText);
        Assert.Equal("推门而入", vm.InputText);

        ws.Dispose();
    }

    [AvaloniaFact]
    public void Evidence_Flags_From_Checks_Changes_And_Memories()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        var msg = new StoryMessage
        {
            Id = "n1",
            Role = "assistant",
            Blocks = new List<TextBlock> { new() { Text = "旁白" } },
            Checks = new List<CheckResult>
            {
                new() { RollId = "r1", ActionId = "a1", Attribute = "敏捷", Natural = 12, AttributeModifier = 2, Total = 14, Dc = 12, Outcome = "success" },
            },
            Changes = new List<StateChange> { new() { Kind = "relation", Label = "信任", Text = "+1", Delta = 1 } },
        };

        var vm = new MessageViewModel(msg, 0, true, ws.Stage, "阿雾", "旅人", new Dictionary<string, string> { ["a1"] = "闪避" });

        Assert.True(vm.HasEvidence);
        Assert.True(vm.HasEvidenceLine);
        Assert.True(vm.HasEvidenceDetail);
        Assert.True(vm.HasChecks);
        Assert.Equal("闪避", vm.Checks[0].Label);
        Assert.True(vm.Checks[0].IsSuccess);
        ws.Dispose();
    }

    [AvaloniaFact]
    public void Composer_Send_Disabled_When_Empty()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        Assert.False(ws.Stage.Composer.CanSend);
        ws.Stage.Composer.Text = "你好";
        Assert.True(ws.Stage.Composer.CanSend);
        ws.Dispose();
    }

    [AvaloniaFact]
    public void MessageView_Lays_Out_With_Data()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        var msg = new StoryMessage
        {
            Id = "n1",
            Role = "assistant",
            TurnNumber = 1,
            InputText = "我走进酒馆",
            Blocks = new List<TextBlock>
            {
                new() { Kind = "narration", Text = "灯火摇曳。" },
                new() { Kind = "dialogue", Text = "「你来了。」" },
            },
            Options = new List<OptionView> { new() { OptionId = "o1", Intent = "clever", Text = "试探他的来意" } },
            Changes = new List<StateChange> { new() { Kind = "relation", Label = "信任", Text = "+1", Delta = 1 } },
        };
        var vm = new MessageViewModel(msg, 0, isLast: true, ws.Stage, "阿雾", "旅人", new Dictionary<string, string>());

        var view = new MessageView { DataContext = vm };
        var window = new Window { Content = view, Width = 840, Height = 900 };
        window.Show();
        Dispatcher.UIThread.RunJobs();
        Assert.True(view.Bounds.Height > 0);
        window.Close();
        ws.Dispose();
    }

    [AvaloniaFact]
    public void StageView_Lays_Out_Empty()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        var view = new StageView { DataContext = ws.Stage };
        var window = new Window { Content = view, Width = 1000, Height = 700 };
        window.Show();
        Dispatcher.UIThread.RunJobs();
        Assert.True(view.Bounds.Width > 0);
        window.Close();
        ws.Dispose();
    }
}

public class NarrativeTextTests
{
    [Fact]
    public void ReplaceMacros_Expands_Char_User_And_Start()
    {
        var input = "{{char}} 对 {{user}} 说 <START>";
        var output = NarrativeText.ReplaceMacros(input, "阿雾", null);
        Assert.Contains("阿雾", output);
        Assert.Contains("旅人", output);
        Assert.Contains("示例对白开始", output);
        Assert.DoesNotContain("{{", output);
    }

    [Fact]
    public void ReplaceMacros_Falls_Back_When_Names_Empty()
    {
        var output = NarrativeText.ReplaceMacros("{{char}}/{{user}}", "", "");
        Assert.Equal(NarrativeText.DefaultCharacterName + "/" + NarrativeText.DefaultPlayerName, output);
    }
}
