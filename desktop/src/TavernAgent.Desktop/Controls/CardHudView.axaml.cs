using Avalonia.Controls;
using TavernAgent.Desktop.Services;

namespace TavernAgent.Desktop.Controls;

/// <summary>Renders a card state panel or scene chip (web: CardHUD).</summary>
public partial class CardHudView : UserControl
{
    public CardHudView()
    {
        InitializeComponent();
        // 卡片主题表面：套用当前卡的主题 token（accent/bg/ink），作用域仅本控件。
        CardThemeSync.Attach(this);
    }
}
