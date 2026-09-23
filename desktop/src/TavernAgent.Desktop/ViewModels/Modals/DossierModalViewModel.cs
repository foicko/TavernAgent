using System.Collections.ObjectModel;
using System.ComponentModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using TavernAgent.Core.Stores;

namespace TavernAgent.Desktop.ViewModels.Modals;

public sealed record DossierTab(string Title, string Content)
{
    public bool HasContent => !string.IsNullOrWhiteSpace(Content);
}

/// <summary>Character dossier modal (web: DossierModal).</summary>
public sealed partial class DossierModalViewModel : WorkspaceChildViewModel
{
    private bool _disposed;

    public DossierModalViewModel(WorkspaceViewModel workspace) : base(workspace)
    {
        Workspace.Ui.PropertyChanged += OnUiChanged;
        Story.PropertyChanged += OnStoryChanged;
        Rebuild();
    }

    public StoryStore Story => Workspace.Story;

    public CharacterPresentation Character { get; private set; } = CharacterPresentation.Resolve(null, null);
    public ObservableCollection<DossierTab> Tabs { get; } = new();
    [ObservableProperty] private DossierTab? _selectedTab;

    public bool IsOpen => Workspace.Ui.DossierModalOpen;

    [RelayCommand] private void Close() => Workspace.Ui.SetDossierModalOpen(false);

    private void OnUiChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName == nameof(UiStore.DossierModalOpen))
        {
            var dispatch = Avalonia.Threading.Dispatcher.UIThread;
            void Apply()
            {
                OnPropertyChanged(nameof(IsOpen));
                if (IsOpen) Rebuild();
            }
            if (dispatch.CheckAccess()) Apply();
            else dispatch.Post(Apply);
        }
    }

    private void OnStoryChanged(object? sender, PropertyChangedEventArgs e)
    {
        if (e.PropertyName == nameof(StoryStore.View))
        {
            var dispatch = Avalonia.Threading.Dispatcher.UIThread;
            if (dispatch.CheckAccess()) Rebuild();
            else dispatch.Post(Rebuild);
        }
    }

    public void Rebuild()
    {
        Character = CharacterPresentation.Resolve(Workspace.Ui.ActiveCharKey, Story.View);
        OnPropertyChanged(nameof(Character));

        var selectedIndex = SelectedTab is null ? 0 : Tabs.IndexOf(SelectedTab);
        Tabs.Clear();
        Tabs.Add(new DossierTab("人设与性格", Join(Character.Description, Character.Personality)));
        Tabs.Add(new DossierTab("世界与场景", Character.Scenario));
        Tabs.Add(new DossierTab("对话范例", Character.MesExample));
        Tabs.Add(new DossierTab("系统规则", Character.SystemPrompt));
        Tabs.Add(new DossierTab("创作者说明", Character.CreatorNotes));
        SelectedTab = Tabs[Math.Clamp(selectedIndex, 0, Tabs.Count - 1)];
    }

    private static string Join(string a, string b)
        => string.IsNullOrWhiteSpace(b) ? a : (string.IsNullOrWhiteSpace(a) ? b : a + "\n\n" + b);

    public override void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        Workspace.Ui.PropertyChanged -= OnUiChanged;
        Story.PropertyChanged -= OnStoryChanged;
    }
}
