using System.Collections.ObjectModel;
using System.ComponentModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Dtos;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Modals;

/// <summary>Promises / contracts modal (web: PromisesModal).</summary>
public sealed partial class PromisesModalViewModel : WorkspaceChildViewModel
{
    private bool _disposed;

    public PromisesModalViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        Workspace.Ui.PropertyChanged += OnUiChanged;
        Story.PropertyChanged += OnStoryChanged;
        Rebuild();
    }

    public StoryStore Story => Workspace.Story;

    public ObservableCollection<PromiseRowViewModel> Promises { get; } = new();
    [ObservableProperty] private string _characterName = "";

    public bool IsOpen => Workspace.Ui.PromisesModalOpen;
    public bool IsEmpty => Promises.Count == 0;

    [RelayCommand] private void Close() => Workspace.Ui.SetPromisesModalOpen(false);

    private void OnUiChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName == nameof(UiStore.PromisesModalOpen))
        {
            OnPropertyChanged(nameof(IsOpen));
            if (IsOpen) Rebuild();
        }
    }

    private void OnStoryChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName is nameof(StoryStore.View) or nameof(StoryStore.Hud)) Rebuild();
    }

    public void Rebuild()
    {
        CharacterName = CharacterPresentation.Resolve(Workspace.Ui.ActiveCharKey, Story.View).ShortName;
        OnPropertyChanged(nameof(CharacterName));
        Promises.Clear();
        foreach (var p in Story.Hud?.Promises?.Values ?? Enumerable.Empty<PromiseItem>())
            Promises.Add(new PromiseRowViewModel(p));
        OnPropertyChanged(nameof(IsEmpty));
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        Workspace.Ui.PropertyChanged -= OnUiChanged;
        Story.PropertyChanged -= OnStoryChanged;
    }
}

public sealed class PromiseRowViewModel
{
    public PromiseRowViewModel(PromiseItem p)
    {
        Content = p.Content;
        SourceNodeId = p.SourceNodeId;
        (StateLabel, StateClass) = p.State switch
        {
            "active" => ("誓约生效中", "active"),
            "fulfilled" => ("已践行达成", "done"),
            "broken" => ("已违背", "broken"),
            "released" => ("已解除", "released"),
            _ => ("口头提议", "proposed"),
        };
    }

    public string Content { get; }
    public string SourceNodeId { get; }
    public string StateLabel { get; }
    public string StateClass { get; }
}
