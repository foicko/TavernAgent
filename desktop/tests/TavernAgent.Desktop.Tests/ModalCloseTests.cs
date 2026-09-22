using System.Linq;
using Avalonia.Controls;
using Avalonia.Headless.XUnit;
using Avalonia.Input;
using Avalonia.Threading;
using Avalonia.VisualTree;
using TavernAgent.Core.Stores;
using TavernAgent.Desktop.Controls;
using TavernAgent.Desktop.ViewModels;
using TavernAgent.Desktop.Views.Modals;
using Xunit;

namespace TavernAgent.Desktop.Tests;

public class ModalCloseTests
{
    [AvaloniaFact]
    public void Dossier_Modal_Closes_Via_Escape_And_Command()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        var view = new DossierModalView { DataContext = ws.Dossier };
        var window = new Window { Content = view, Width = 1100, Height = 760 };
        window.Show();
        Dispatcher.UIThread.RunJobs();

        var modal = view.GetVisualDescendants().OfType<Modal>().First();
        Assert.False(modal.IsVisible);

        ws.Ui.SetDossierModalOpen(true);
        Dispatcher.UIThread.RunJobs();
        Assert.True(modal.IsVisible);

        modal.RaiseEvent(new KeyEventArgs { RoutedEvent = InputElement.KeyDownEvent, Key = Key.Escape });
        Dispatcher.UIThread.RunJobs();
        Assert.False(ws.Ui.DossierModalOpen);
        Assert.False(modal.IsVisible);

        window.Close();
        ws.Dispose();
    }

    [AvaloniaFact]
    public void Memory_And_Promises_Modals_Close_Via_Command()
    {
        var ws = new WorkspaceViewModel(new UiStore());
        var memory = new MemoryModalView { DataContext = ws.MemoryModal };
        var promises = new PromisesModalView { DataContext = ws.Promises };
        var window = new Window
        {
            Content = new StackPanel { Children = { memory, promises } },
            Width = 1100,
            Height = 760,
        };
        window.Show();
        Dispatcher.UIThread.RunJobs();

        ws.Ui.SetMemoryModalOpen(true);
        ws.Ui.SetPromisesModalOpen(true);
        Dispatcher.UIThread.RunJobs();

        ws.MemoryModal.CloseCommand.Execute(null);
        ws.Promises.CloseCommand.Execute(null);
        Dispatcher.UIThread.RunJobs();

        Assert.False(ws.Ui.MemoryModalOpen);
        Assert.False(ws.Ui.PromisesModalOpen);
        Assert.False(memory.GetVisualDescendants().OfType<Modal>().First().IsVisible);
        Assert.False(promises.GetVisualDescendants().OfType<Modal>().First().IsVisible);

        window.Close();
        ws.Dispose();
    }
}
