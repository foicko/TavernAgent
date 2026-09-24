namespace TavernAgent.Desktop.ViewModels.Right;

public sealed record InventoryActViewModel(string Key, string Label, string Text);

/// <summary>Player-owned inventory item (web: .item-slot + InventoryActionBubble).</summary>
public sealed class InventoryItemViewModel
{
    public InventoryItemViewModel(string id, string name, string status, string icon, IReadOnlyList<InventoryActViewModel> acts)
    {
        Id = id;
        Name = name;
        Status = status;
        Icon = icon;
        Acts = acts;
    }

    public string Id { get; }
    public string Name { get; }
    public string Status { get; }
    public string Icon { get; }
    public IReadOnlyList<InventoryActViewModel> Acts { get; }
}
