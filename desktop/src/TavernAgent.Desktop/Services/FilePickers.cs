using Avalonia;
using Avalonia.Controls;
using Avalonia.Platform.Storage;

namespace TavernAgent.Desktop.Services;

/// <summary>File open/save helpers for .tavernpack export/import and card files.</summary>
public static class FilePickers
{
    private static readonly FilePickerFileType PackType = new("Tavern 剧情包")
    {
        Patterns = new[] { "*.tavernpack", "*.zip" },
    };

    private static readonly FilePickerFileType CardType = new("角色卡")
    {
        Patterns = new[] { "*.png", "*.json" },
    };

    public static async Task<byte[]?> PickPackAsync(Visual owner)
    {
        var top = TopLevel.GetTopLevel(owner);
        if (top is null) return null;
        var files = await top.StorageProvider.OpenFilePickerAsync(new FilePickerOpenOptions
        {
            Title = "导入剧情包",
            AllowMultiple = false,
            FileTypeFilter = new[] { PackType },
        });
        var file = files.FirstOrDefault();
        if (file is null) return null;
        await using var stream = await file.OpenReadAsync();
        using var ms = new MemoryStream();
        await stream.CopyToAsync(ms);
        return ms.ToArray();
    }

    public static async Task<byte[]?> PickCardAsync(Visual owner)
    {
        var picked = await PickCardFileAsync(owner);
        return picked?.Data;
    }

    public static async Task<(string Name, byte[] Data)?> PickCardFileAsync(Visual owner)
    {
        var top = TopLevel.GetTopLevel(owner);
        if (top is null) return null;
        var files = await top.StorageProvider.OpenFilePickerAsync(new FilePickerOpenOptions
        {
            Title = "导入角色卡",
            AllowMultiple = false,
            FileTypeFilter = new[] { CardType },
        });
        var file = files.FirstOrDefault();
        if (file is null) return null;
        await using var stream = await file.OpenReadAsync();
        using var ms = new MemoryStream();
        await stream.CopyToAsync(ms);
        return (file.Name, ms.ToArray());
    }

    public static async Task SavePackAsync(Visual owner, string suggestedName, byte[] data)
    {
        var top = TopLevel.GetTopLevel(owner);
        if (top is null) return;
        var file = await top.StorageProvider.SaveFilePickerAsync(new FilePickerSaveOptions
        {
            Title = "导出剧情包",
            SuggestedFileName = suggestedName,
            DefaultExtension = "tavernpack",
            FileTypeChoices = new[] { PackType },
        });
        if (file is null) return;
        await using var stream = await file.OpenWriteAsync();
        await stream.WriteAsync(data);
    }
}
