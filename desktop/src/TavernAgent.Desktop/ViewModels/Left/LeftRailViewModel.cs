using System.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Left;

/// <summary>Left rail container: dock, tab header and four panels (web: LeftRail).</summary>
public sealed partial class LeftRailViewModel : WorkspaceChildViewModel
{
    private bool _disposed;

    public LeftRailViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        Tree = new BranchTreeViewModel(workspace);
        Characters = new CharactersPanelViewModel(workspace);
        Lore = new LorePanelViewModel(workspace);
        Sessions = new SessionsPanelViewModel(workspace);
        StoryMap = new StoryMapViewModel(workspace);
        workspace.Ui.PropertyChanged += OnUiChanged;
    }

    public BranchTreeViewModel Tree { get; }
    public CharactersPanelViewModel Characters { get; }
    public LorePanelViewModel Lore { get; }
    public SessionsPanelViewModel Sessions { get; }
    public StoryMapViewModel StoryMap { get; }

    public bool IsCollapsed => Workspace.IsLeftCollapsed;
    public bool ShowFullView => true;
    public bool ShowDock => false;
    public bool IsFlyoutOpen => Workspace.Ui.FlyoutOpen;

    public LeftSubtab Tab => Workspace.Ui.LeftTab;
    public bool IsTreeTab => Tab == LeftSubtab.Tree;
    public bool IsCharactersTab => Tab == LeftSubtab.Characters;
    public bool IsLoreTab => Tab == LeftSubtab.Lore;
    public bool IsSessionsTab => Tab == LeftSubtab.Sessions;

    public bool IsTreeTabActive => IsTreeTab && IsFlyoutOpen;
    public bool IsCharactersTabActive => IsCharactersTab && IsFlyoutOpen;
    public bool IsLoreTabActive => IsLoreTab && IsFlyoutOpen;
    public bool IsSessionsTabActive => IsSessionsTab && IsFlyoutOpen;

    /// <summary>Remembered tab while the flyout is closed — a quiet "you were here" marker.</summary>
    public bool IsTreeTabVisited => IsTreeTab && !IsFlyoutOpen;
    public bool IsCharactersTabVisited => IsCharactersTab && !IsFlyoutOpen;
    public bool IsLoreTabVisited => IsLoreTab && !IsFlyoutOpen;
    public bool IsSessionsTabVisited => IsSessionsTab && !IsFlyoutOpen;

    public string PanelTitle => Tab switch
    {
        LeftSubtab.Characters => "角色卡库",
        LeftSubtab.Lore => "世界书",
        LeftSubtab.Sessions => "历史会话",
        _ => "因果分支",
    };

    public bool IsDirectorActive => Workspace.Ui.DirectorOpen;
    public bool IsDarkTheme => Workspace.Ui.Theme == ThemeMode.Dark;
    public bool HasStory => Workspace.Story.View is not null;

    [RelayCommand] private void Collapse() => Workspace.Ui.CloseFlyout();
    [RelayCommand] private void ShowTree() => Activate(LeftSubtab.Tree);
    [RelayCommand] private void ShowCharacters() => Activate(LeftSubtab.Characters);
    [RelayCommand] private void ShowLore() => Activate(LeftSubtab.Lore);
    [RelayCommand] private void ShowSessions() => Activate(LeftSubtab.Sessions);

    [RelayCommand] private void OpenDirector() => Workspace.Stage.OpenDirector();
    [RelayCommand] private void OpenSettings() => Workspace.Ui.SetSettingsOpen(true);
    [RelayCommand] private void OpenPalette() => Workspace.Ui.TogglePalette();
    [RelayCommand] private void ToggleTheme() => Workspace.Ui.ToggleTheme();

    private void Activate(LeftSubtab tab)
    {
        Workspace.Ui.ToggleFlyout(tab);
        if (Workspace.Ui.FlyoutOpen) OnTabActivated(tab);
    }

    private void OnTabActivated(LeftSubtab tab)
    {
        switch (tab)
        {
            case LeftSubtab.Tree: Tree.Rebuild(); break;
            case LeftSubtab.Characters: _ = Characters.ReloadAsync(); break;
            case LeftSubtab.Lore: _ = Lore.SearchAsync(); break;
            case LeftSubtab.Sessions: Sessions.Rebuild(); break;
        }
    }

    private void OnUiChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName is nameof(UiStore.LeftCollapsed) or nameof(UiStore.LeftTab) or nameof(UiStore.FlyoutOpen)
            or nameof(UiStore.DirectorOpen) or nameof(UiStore.Theme))
        {
            OnPropertyChanged(nameof(IsCollapsed));
            OnPropertyChanged(nameof(ShowFullView));
            OnPropertyChanged(nameof(ShowDock));
            OnPropertyChanged(nameof(IsFlyoutOpen));
            OnPropertyChanged(nameof(IsTreeTabActive));
            OnPropertyChanged(nameof(IsCharactersTabActive));
            OnPropertyChanged(nameof(IsLoreTabActive));
            OnPropertyChanged(nameof(IsSessionsTabActive));
            OnPropertyChanged(nameof(IsTreeTabVisited));
            OnPropertyChanged(nameof(IsCharactersTabVisited));
            OnPropertyChanged(nameof(IsLoreTabVisited));
            OnPropertyChanged(nameof(IsSessionsTabVisited));
            OnPropertyChanged(nameof(PanelTitle));
            OnPropertyChanged(nameof(IsDirectorActive));
            OnPropertyChanged(nameof(IsDarkTheme));
            OnPropertyChanged(nameof(HasStory));
            OnPropertyChanged(nameof(Tab));
            OnPropertyChanged(nameof(IsTreeTab));
            OnPropertyChanged(nameof(IsCharactersTab));
            OnPropertyChanged(nameof(IsLoreTab));
            OnPropertyChanged(nameof(IsSessionsTab));
        }
    }

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        Workspace.Ui.PropertyChanged -= OnUiChanged;
        Tree.Dispose();
        Characters.Dispose();
        Lore.Dispose();
        Sessions.Dispose();
        StoryMap.Dispose();
    }
}
