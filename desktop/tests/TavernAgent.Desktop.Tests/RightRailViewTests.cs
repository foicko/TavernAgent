using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using Avalonia.Threading;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.ViewModels;
using TavernAgent.Desktop.ViewModels.Left;
using TavernAgent.Desktop.ViewModels.Right;
using TavernAgent.Desktop.Views.Modals;
using TavernAgent.Desktop.Views.Right;
using Xunit;

namespace TavernAgent.Desktop.Tests;

public class RightRailViewTests
{
    [AvaloniaFact]
    public void RightRail_Lays_Out_With_Defaults()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        var view = new RightRailView { DataContext = ws.RightRail };
        var window = new Window { Content = view, Width = 1200, Height = 900 };
        window.Show();
        Dispatcher.UIThread.RunJobs();

        Assert.True(view.Bounds.Width > 0);
        Assert.NotNull(ws.RightRail.Character);
        Assert.NotNull(ws.RightRail.MeterAffection);

        ws.RightRail.OpenDossierCommand.Execute(null);
        Assert.True(ws.Ui.DossierModalOpen);
        ws.RightRail.SetSmileCommand.Execute(null);
        Assert.True(ws.RightRail.IsSmileExpr);

        window.Close();
        ws.Dispose();
    }

    [AvaloniaFact]
    public void Modals_Open_And_Close()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        var dossier = new DossierModalView { DataContext = ws.Dossier };
        var window = new Window { Content = dossier, Width = 1000, Height = 700 };
        window.Show();
        Dispatcher.UIThread.RunJobs();

        ws.Ui.SetDossierModalOpen(true);
        Dispatcher.UIThread.RunJobs();
        Assert.True(ws.Dossier.IsOpen);
        Assert.NotEmpty(ws.Dossier.Tabs);

        ws.Dossier.CloseCommand.Execute(null);
        Assert.False(ws.Ui.DossierModalOpen);

        window.Close();
        ws.Dispose();
    }
}

public class RailLogicTests
{
    [Fact]
    public void Meter_Affection_Percent_Formula()
    {
        Assert.Equal(50, MeterViewModel.AffectionPercent(0));
        Assert.Equal(100, MeterViewModel.AffectionPercent(100));
        Assert.Equal(0, MeterViewModel.AffectionPercent(-100));
        Assert.Equal(75, MeterViewModel.AffectionPercent(50));
    }

    [Fact]
    public void Meter_Tone_Drives_Styling_Flags()
    {
        Assert.True(MeterViewModel.Affection(12).IsAffection);
        Assert.False(MeterViewModel.Affection(12).IsTrust);
        Assert.True(MeterViewModel.Trust(40).IsTrust);
        Assert.True(MeterViewModel.Alert(7).IsAlert);
        Assert.False(MeterViewModel.Alert(7).IsAffection);
    }

    [Fact]
    public void Meter_Values_Keep_Their_Unit_Conventions()
    {
        Assert.Equal("+12", MeterViewModel.Affection(12).ValueText);
        Assert.Equal("-8", MeterViewModel.Affection(-8).ValueText);
        Assert.Equal("40%", MeterViewModel.Trust(40).ValueText);
        Assert.Equal("7%", MeterViewModel.Alert(7).ValueText);
    }

    [Fact]
    public void Session_Time_Formatting()
    {
        Assert.Equal("最近", SessionRowViewModel.FormatSessionTime(null));
        Assert.Equal("刚刚", SessionRowViewModel.FormatSessionTime(DateTimeOffset.UtcNow.AddSeconds(-10).ToString("O")));
        Assert.Equal("5 分钟前", SessionRowViewModel.FormatSessionTime(DateTimeOffset.UtcNow.AddMinutes(-5).ToString("O")));
        Assert.Equal("2 小时前", SessionRowViewModel.FormatSessionTime(DateTimeOffset.UtcNow.AddHours(-2).ToString("O")));
        Assert.Equal("3 天前", SessionRowViewModel.FormatSessionTime(DateTimeOffset.UtcNow.AddDays(-3).ToString("O")));
    }

    [Fact]
    public void Checkpoint_Parses_Xml_And_Falls_Back_To_Text()
    {
        var vm = new StoryCheckpointViewModel();
        vm.Load("<story_checkpoint><narrative_arc>推进</narrative_arc><open_loops>悬念</open_loops>" +
                "<character_dynamics><mindset character=\"阿雾\">不安</mindset><hidden_tension>暗流</hidden_tension></character_dynamics>" +
                "<milestones>里程碑</milestones></story_checkpoint>");
        Assert.True(vm.HasAny);
        Assert.Equal("推进", vm.NarrativeArc);
        Assert.Equal("悬念", vm.OpenLoops);
        Assert.Equal("暗流", vm.HiddenTension);
        Assert.Single(vm.Mindsets);
        Assert.Equal("阿雾", vm.Mindsets[0].Character);

        var plain = new StoryCheckpointViewModel();
        plain.Load("没有 XML 的纯文本");
        Assert.Equal("没有 XML 的纯文本", plain.NarrativeArc);
    }
}
