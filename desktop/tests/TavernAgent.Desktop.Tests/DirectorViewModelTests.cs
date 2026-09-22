using System.Linq;
using Avalonia.Headless.XUnit;
using Avalonia.Threading;
using Avalonia.VisualTree;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.ViewModels;
using Xunit;

namespace TavernAgent.Desktop.Tests;

public class DirectorViewModelTests
{
    [AvaloniaFact]
    public void Director_Requires_A_Session_To_Open()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        ws.Stage.OpenDirector();
        Dispatcher.UIThread.RunJobs();
        Assert.False(ws.Ui.DirectorOpen);
        Assert.Contains("先开启一个故事", ws.Ui.ToastMessage ?? "");
        ws.Dispose();
    }

    [AvaloniaFact]
    public void Director_Beat_Editing_And_Validation()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        var director = ws.Stage.Director;

        Assert.False(director.CanActivate);

        director.AddBeatCommand.Execute(null);
        Assert.Single(director.Beats);
        Assert.False(director.CanActivate);

        var beat = director.Beats[0];
        beat.Title = "第一幕";
        beat.Instruction = "推进雨夜客栈的暗流";
        beat.CompletionCriteria = "玩家与掌柜发生冲突";
        director.Title = "测试大纲";
        Assert.True(director.CanActivate);

        director.AddBeatCommand.Execute(null);
        Assert.Equal(2, director.Beats.Count);
        Assert.Equal(1, director.Beats[0].Number);
        Assert.Equal(2, director.Beats[1].Number);

        director.MoveBeatUpCommand.Execute(director.Beats[1]);
        Assert.Equal("第一幕", director.Beats[1].Title);
        Assert.Equal(1, director.Beats[0].Number);

        director.DeleteBeatCommand.Execute(director.Beats[0]);
        Assert.Single(director.Beats);
        Assert.Equal(1, director.Beats[0].Number);

        ws.Dispose();
    }

    [AvaloniaFact]
    public void Palette_Exposes_Director_Command()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        ws.Ui.SetPaletteOpen(true);
        Dispatcher.UIThread.RunJobs();
        Assert.Contains(ws.Palette.Results, r => r.Id == "panel:director");
        ws.Dispose();
    }

    [AvaloniaFact]
    public void Director_Workspace_View_Opens_And_Closes()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        var view = new TavernAgent.Desktop.Views.Stage.DirectorWorkspaceView { DataContext = ws.Stage.Director };
        var window = new Avalonia.Controls.Window { Content = view, Width = 1280, Height = 900 };
        window.Show();
        Dispatcher.UIThread.RunJobs();

        var modal = view.GetVisualDescendants().OfType<TavernAgent.Desktop.Controls.Modal>().First();
        Assert.False(modal.IsVisible);

        ws.Ui.SetDirectorOpen(true);
        Dispatcher.UIThread.RunJobs();
        Assert.True(modal.IsVisible);

        ws.Stage.Director.CloseCommand.Execute(null);
        Dispatcher.UIThread.RunJobs();
        Assert.False(ws.Ui.DirectorOpen);

        window.Close();
        ws.Dispose();
    }
}
