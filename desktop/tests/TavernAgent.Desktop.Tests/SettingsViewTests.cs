using System.Linq;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using Avalonia.Threading;
using Avalonia.VisualTree;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.Controls;
using TavernAgent.Desktop.ViewModels;
using TavernAgent.Desktop.ViewModels.Modals.Settings;
using TavernAgent.Desktop.Views.Modals.Settings;
using Xunit;

namespace TavernAgent.Desktop.Tests;

public class SettingsViewTests
{
    [AvaloniaFact]
    public void Settings_Modal_Opens_And_Switches_Tabs()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        var view = new SettingsModalView { DataContext = ws.SettingsModal };
        var window = new Window { Content = view, Width = 1200, Height = 900 };
        window.Show();
        Dispatcher.UIThread.RunJobs();

        var modal = view.GetVisualDescendants().OfType<Modal>().First();
        Assert.False(modal.IsVisible);

        ws.Ui.SetSettingsOpen(true);
        Dispatcher.UIThread.RunJobs();
        Assert.True(ws.SettingsModal.IsOpen);
        Assert.True(modal.IsVisible);
        Assert.Equal(4, ws.SettingsModal.Tabs.Count);
        Assert.IsType<ModelSettingsPageViewModel>(ws.SettingsModal.Page);

        ws.SettingsModal.SelectTabCommand.Execute(ws.SettingsModal.Tabs[1]);
        Dispatcher.UIThread.RunJobs();
        Assert.IsType<TtsSettingsPageViewModel>(ws.SettingsModal.Page);

        ws.SettingsModal.SelectTabCommand.Execute(ws.SettingsModal.Tabs[3]);
        Dispatcher.UIThread.RunJobs();
        Assert.IsType<EmbeddingSettingsPageViewModel>(ws.SettingsModal.Page);

        window.Close();
        ws.Dispose();
    }

    [AvaloniaFact]
    public void Connection_Form_Opens_And_Validates()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        var page = ws.SettingsModal.Models;

        page.NewConnectionCommand.Execute(null);
        var form = page.Editing;
        Assert.NotNull(form);
        Assert.True(page.IsEditing);
        Assert.True(form!.IsNew);

        // Blank model/name are invalid → save keeps the form open and reports errors.
        form.SaveCommand.Execute(null);
        Dispatcher.UIThread.RunJobs();
        Assert.True(page.IsEditing);
        Assert.NotNull(form.ModelError);
        Assert.NotNull(form.NameError);

        // Preset fills provider defaults and suggests a name.
        var preset = ProviderPresets.All.First(p => p.Id == "deepseek");
        form.ApplyPresetCommand.Execute(preset);
        Assert.Equal("https://api.deepseek.com", form.BaseUrl);
        Assert.False(string.IsNullOrEmpty(form.Model));
        Assert.False(string.IsNullOrEmpty(form.Name));

        page.CancelEdit();
        Assert.False(page.IsEditing);
        Assert.Null(page.Editing);

        ws.Dispose();
    }
}
