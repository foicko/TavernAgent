package dev.tavernagent.mobile.ui.nav

/** 全局导航路由。参数以路径段形式拼接，避免 query 编码问题。 */
object Routes {
    const val CONNECT = "connect"

    /** 聊天列表：会话在 UI 上就是「与某个角色的对话」。 */
    const val CHATS = "chats"

    /** 角色库：角色卡在 UI 上就是「可对话的角色」。 */
    const val CARDS = "cards"

    const val SETTINGS = "settings"
    const val MODELS = "models"
    const val ARCHIVE = "archive"

    const val STORY = "story/{sessionId}/{branchId}/{viewNodeId}"
    const val BRANCH_TREE = "branch/{sessionId}/{branchId}"
    const val CHARACTERS = "characters/{sessionId}/{branchId}"
    const val CHARACTER_DETAIL = "character/{cardId}"
    const val MEMORY = "memory/{sessionId}/{branchId}"
    const val DIRECTOR = "director/{sessionId}/{branchId}"

    const val NO_VIEW_NODE = "_"
    const val AUTO_BRANCH = "_"

    fun story(sessionId: String, branchId: String, viewNodeId: String = NO_VIEW_NODE) =
        "story/$sessionId/$branchId/$viewNodeId"

    fun branchTree(sessionId: String, branchId: String) = "branch/$sessionId/$branchId"
    fun characters(sessionId: String, branchId: String) = "characters/$sessionId/$branchId"
    fun characterDetail(cardId: String) = "character/$cardId"
    fun memory(sessionId: String, branchId: String) = "memory/$sessionId/$branchId"
    fun director(sessionId: String, branchId: String) = "director/$sessionId/$branchId"

    const val ARG_SESSION = "sessionId"
    const val ARG_BRANCH = "branchId"
    const val ARG_VIEW_NODE = "viewNodeId"
    const val ARG_CARD = "cardId"
}
