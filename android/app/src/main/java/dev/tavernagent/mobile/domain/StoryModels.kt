package dev.tavernagent.mobile.domain

import dev.tavernagent.mobile.data.remote.ApiJson
import dev.tavernagent.mobile.data.remote.dto.InFlightDto
import dev.tavernagent.mobile.data.remote.dto.OptionDto
import dev.tavernagent.mobile.data.remote.dto.PlotNodeDto
import dev.tavernagent.mobile.data.remote.dto.SessionViewDto
import dev.tavernagent.mobile.data.remote.dto.TextBlockDto
import dev.tavernagent.mobile.data.remote.dto.TurnContentDto
import dev.tavernagent.mobile.data.remote.dto.UiHintDto
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement

/** 回合相位机（与 Web 端一致）。 */
enum class TurnPhase {
    IDLE,
    GENERATING,
    TRUNCATED,
    AWAITING_CONFIRM,
    COMMITTED,
    FAILED,
    ;

    val isBusy: Boolean get() = this == GENERATING || this == TRUNCATED || this == AWAITING_CONFIRM || this == COMMITTED
}

/** 一条渲染用的故事消息（由节点派生，不在本地推断剧情状态）。 */
data class StoryMessage(
    val nodeId: String,
    val parentId: String,
    val kind: String,
    val isRoot: Boolean,
    val turnNumber: Int,
    val inputText: String,
    val inputNote: String?,
    val blocks: List<TextBlockDto>,
    val options: List<OptionDto>,
    val uiHints: List<UiHintDto>,
    val changes: List<JsonElement>,
    val suppressedOptions: Int,
    val thinking: String? = null,
    val thinkingSeconds: Int? = null,
    val createdAt: String? = null,
)

@Serializable
private data class RootContent(
    val openingText: String = "",
    val openingVariantId: String? = null,
)

/** 从服务端视图派生的消息列表；只读展示，不改变任何分支状态。 */
object StoryMessages {

    fun fromView(view: SessionViewDto, thinking: Map<String, String> = emptyMap(), seconds: Map<String, Int> = emptyMap()): List<StoryMessage> {
        val nodes = view.nodes.sortedWith(compareBy({ it.depth }, { it.turnNumber }))
        val out = ArrayList<StoryMessage>(nodes.size)
        for (node in nodes) {
            when (node.kind) {
                "root" -> out.add(rootMessage(node))
                "turn" -> node.toStoryMessage(thinking, seconds)?.let { out.add(it) }
                else -> Unit // 记忆整理/导演事件等节点在分支树中展示，不进入叙事流
            }
        }
        return out
    }

    private fun rootMessage(node: PlotNodeDto): StoryMessage {
        val root = ApiJson.decodeOrNull<RootContent>(node.contentJson)
        val text = root?.openingText.orEmpty()
        val blocks = if (text.isBlank()) emptyList() else listOf(TextBlockDto(kind = "narration", text = text))
        return StoryMessage(
            nodeId = node.nodeId,
            parentId = node.parentId,
            kind = node.kind,
            isRoot = true,
            turnNumber = 0,
            inputText = "",
            inputNote = null,
            blocks = blocks,
            options = emptyList(),
            uiHints = emptyList(),
            changes = emptyList(),
            suppressedOptions = 0,
            createdAt = node.createdAt,
        )
    }

    private fun PlotNodeDto.toStoryMessage(
        thinking: Map<String, String>,
        seconds: Map<String, Int>,
    ): StoryMessage? {
        val content: TurnContentDto = turnContent() ?: return null
        return StoryMessage(
            nodeId = nodeId,
            parentId = parentId,
            kind = kind,
            isRoot = false,
            turnNumber = turnNumber,
            inputText = content.inputText,
            inputNote = content.inputNote,
            blocks = content.blocks,
            options = content.options,
            uiHints = content.uiHints,
            changes = content.changes,
            suppressedOptions = content.suppressedOptions,
            thinking = thinking[nodeId] ?: content.provenance?.turnId?.let { thinking[it] },
            thinkingSeconds = seconds[nodeId],
            createdAt = createdAt,
        )
    }
}

/** 流式草稿块：按 seq 定位，缺位补空。 */
data class DraftBlock(
    val seq: Int,
    val kind: String = "narration",
    val speakerId: String? = null,
    val text: String = "",
)

/** 草稿合并规则与 Web 端 `mergeInFlightDraft` 一致：只允许让文本变长。 */
object DraftReducer {

    fun applyDelta(blocks: List<DraftBlock>, seq: Int, kind: String?, speakerId: String?, delta: String): List<DraftBlock> {
        if (delta.isEmpty()) return blocks
        val out = blocks.toMutableList()
        if (seq <= 0) {
            val last = out.lastOrNull()
            if (last == null) {
                out.add(DraftBlock(seq = 1, kind = kind ?: "narration", speakerId = speakerId, text = delta))
            } else {
                out[out.lastIndex] = last.copy(text = last.text + delta)
            }
            return out
        }
        while (out.size < seq) {
            out.add(DraftBlock(seq = out.size + 1))
        }
        val index = seq - 1
        val current = out[index]
        out[index] = current.copy(
            kind = if (current.text.isEmpty()) (kind ?: current.kind) else current.kind,
            speakerId = current.speakerId ?: speakerId,
            text = current.text + delta,
        )
        return out
    }

    fun applyBlockAppended(blocks: List<DraftBlock>, seq: Int, kind: String?, speakerId: String?, text: String): List<DraftBlock> {
        if (seq <= 0) return applySnapshot(blocks, seq = blocks.size, kind = kind, speakerId = speakerId, text = text)
        val out = blocks.toMutableList()
        while (out.size < seq) out.add(DraftBlock(seq = out.size + 1))
        out[seq - 1] = DraftBlock(seq = seq, kind = kind ?: "narration", speakerId = speakerId, text = text)
        return out
    }

    /** 在途快照：补齐断线期间丢失的行内增量，只允许变长。 */
    fun applyInFlight(blocks: List<DraftBlock>, inFlight: InFlightDto?): List<DraftBlock> {
        if (inFlight == null || inFlight.text.isEmpty()) return blocks
        return applySnapshot(blocks, inFlight.seq, inFlight.kind, inFlight.speakerId, inFlight.text)
    }

    private fun applySnapshot(blocks: List<DraftBlock>, seq: Int, kind: String?, speakerId: String?, text: String): List<DraftBlock> {
        val out = blocks.toMutableList()
        if (seq <= 0) {
            out.add(DraftBlock(seq = out.size + 1, kind = kind ?: "narration", speakerId = speakerId, text = text))
            return out
        }
        while (out.size < seq) out.add(DraftBlock(seq = out.size + 1))
        val index = seq - 1
        val current = out[index]
        if (current.text.length >= text.length) return blocks
        out[index] = DraftBlock(
            seq = seq,
            kind = kind ?: current.kind,
            speakerId = speakerId ?: current.speakerId,
            text = text,
        )
        return out
    }
}
