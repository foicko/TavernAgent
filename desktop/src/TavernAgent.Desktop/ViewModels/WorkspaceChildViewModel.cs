namespace TavernAgent.Desktop.ViewModels;

/// <summary>Base for VMs that live inside the workspace and reference its shared stores.</summary>
public abstract class WorkspaceChildViewModel : ViewModelBase
{
    protected WorkspaceChildViewModel(WorkspaceViewModel workspace)
    {
        Workspace = workspace;
    }

    public WorkspaceViewModel Workspace { get; }
}
