package dev.tavernagent.mobile.util

import kotlinx.coroutines.CancellationException

/**
 * `runCatching` 的协程安全版本。
 *
 * `runCatching` 会把 `CancellationException` 一并捕获为 `Result.failure`，破坏结构化并发：
 * 被取消的加载仍会执行 `onFailure` 并写入陈旧状态（ViewModel 已被销毁/切换会话后仍在写）。
 * 这里让取消继续传播，其余异常照旧转为 `Result.failure`。
 */
inline fun <T> runCatchingCancellable(block: () -> T): Result<T> = try {
    Result.success(block())
} catch (e: CancellationException) {
    throw e
} catch (e: Throwable) {
    Result.failure(e)
}
