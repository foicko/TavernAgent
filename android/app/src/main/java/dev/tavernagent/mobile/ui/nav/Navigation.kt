package dev.tavernagent.mobile.ui.nav

import androidx.navigation.NavController

/**
 * 打开故事页的统一入口。
 *
 * 故事页 / 分支树 / 角色 / 记忆 / 导演之间来回跳转时逐个 push 会让返回栈无限增长，
 * 而且每次都是一个**新的 ViewModel**（重新拉取会话、重置滚动位置）。因此：
 * - `launchSingleTop`：目标已在栈顶且参数相同则复用，不重复创建；
 * - `popUpTo(STORY, inclusive)`：压入新故事前先弹掉旧的故事页——从分支树选节点
 *   回到故事时，返回栈保持"聊表 → 分支树 → 故事"，而不是层层叠故事。
 */
fun NavController.navigateToStory(
    sessionId: String,
    branchId: String,
    viewNodeId: String = Routes.NO_VIEW_NODE,
) {
    navigate(Routes.story(sessionId, branchId, viewNodeId)) {
        popUpTo(Routes.STORY) { inclusive = true }
        launchSingleTop = true
    }
}
