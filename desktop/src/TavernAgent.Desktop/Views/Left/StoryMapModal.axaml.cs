using Avalonia.Controls;
using TavernAgent.Core.Dtos;
using TavernAgent.Desktop.ViewModels.Left;

namespace TavernAgent.Desktop.Views.Left;

public partial class StoryMapModal : UserControl
{
    public StoryMapModal()
    {
        InitializeComponent();
    }

    private void OnBranchSelected(object? sender, SelectionChangedEventArgs e)
    {
        if (DataContext is not StoryMapViewModel vm) return;
        if (sender is ComboBox { SelectedItem: BranchView branch })
            vm.LocateBranchCommand.Execute(branch);
    }
}
