using Avalonia.Controls;
using Avalonia.Input;
using Avalonia.Interactivity;
using Avalonia.Platform.Storage;
using TavernAgent.Desktop.Services;
using TavernAgent.Desktop.ViewModels.Modals;

namespace TavernAgent.Desktop.Views.Modals;

public partial class CharacterImportModalView : UserControl
{
    public CharacterImportModalView()
    {
        InitializeComponent();
        AddHandler(DragDrop.DragOverEvent, OnDragOver);
        AddHandler(DragDrop.DropEvent, OnDrop);
    }

    private async void OnPickClick(object? sender, RoutedEventArgs e)
    {
        if (DataContext is not CharacterImportViewModel vm) return;
        var picked = await FilePickers.PickCardFileAsync(this);
        if (picked is null) return;
        await vm.ImportAsync(picked.Value.Name, picked.Value.Data);
    }

    private static void OnDragOver(object? sender, DragEventArgs e)
    {
        e.DragEffects = e.DataTransfer.Contains(DataFormat.File) ? DragDropEffects.Copy : DragDropEffects.None;
        e.Handled = true;
    }

    private async void OnDrop(object? sender, DragEventArgs e)
    {
        if (DataContext is not CharacterImportViewModel vm) return;
        var file = e.DataTransfer.TryGetFiles()?.OfType<IStorageFile>().FirstOrDefault();
        if (file is null) return;
        await using var stream = await file.OpenReadAsync();
        using var ms = new MemoryStream();
        await stream.CopyToAsync(ms);
        await vm.ImportAsync(file.Name, ms.ToArray());
        e.Handled = true;
    }
}
