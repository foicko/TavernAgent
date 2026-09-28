package dev.tavernagent.mobile.data.local

import android.content.Context
import android.net.Uri
import android.provider.OpenableColumns
import java.io.ByteArrayOutputStream

/** 从 SAF URI 读取角色卡/剧情包字节，并按魔数推断媒体类型。 */
object FilePickers {

    data class PickedFile(val bytes: ByteArray, val mediaType: String, val name: String)

    /** 客户端读取上限：与服务端上传上限无关，避免误选大文件直接把 App OOM。 */
    private const val MAX_PICK_BYTES = 64L * 1024 * 1024

    fun read(context: Context, uri: Uri): PickedFile {
        val declared = sizeOf(context, uri)
        if (declared > MAX_PICK_BYTES) {
            error("文件过大（${declared / (1024 * 1024)}MB），上限 ${MAX_PICK_BYTES / (1024 * 1024)}MB")
        }
        val bytes = context.contentResolver.openInputStream(uri)?.use { stream ->
            // 声明大小可能为未知/0：按上限流式读取并即时拦截超大文件，避免一次性 readBytes()。
            val buffer = ByteArrayOutputStream()
            val chunk = ByteArray(64 * 1024)
            var total = 0L
            while (true) {
                val read = stream.read(chunk)
                if (read < 0) break
                total += read
                if (total > MAX_PICK_BYTES) {
                    error("文件过大，上限 ${MAX_PICK_BYTES / (1024 * 1024)}MB")
                }
                buffer.write(chunk, 0, read)
            }
            buffer.toByteArray()
        } ?: error("无法读取所选文件")
        val name = displayName(context, uri)
        return PickedFile(bytes = bytes, mediaType = sniffMediaType(bytes, name), name = name)
    }

    private fun sizeOf(context: Context, uri: Uri): Long {
        context.contentResolver.query(uri, null, null, null, null)?.use { cursor ->
            val index = cursor.getColumnIndex(OpenableColumns.SIZE)
            if (index >= 0 && cursor.moveToFirst() && !cursor.isNull(index)) return cursor.getLong(index)
        }
        return -1
    }

    fun displayName(context: Context, uri: Uri): String {
        context.contentResolver.query(uri, null, null, null, null)?.use { cursor ->
            val index = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
            if (index >= 0 && cursor.moveToFirst()) return cursor.getString(index) ?: "card"
        }
        return uri.lastPathSegment ?: "card"
    }

    fun sniffMediaType(bytes: ByteArray, name: String = ""): String = when {
        bytes.size >= 8 && bytes[0] == 0x89.toByte() && bytes[1] == 'P'.code.toByte() &&
            bytes[2] == 'N'.code.toByte() && bytes[3] == 'G'.code.toByte() -> "image/png"
        bytes.size >= 4 && bytes[0] == 'P'.code.toByte() && bytes[1] == 'K'.code.toByte() -> "application/zip"
        name.endsWith(".charx", ignoreCase = true) -> "application/zip"
        name.endsWith(".tavernpack", ignoreCase = true) -> "application/zip"
        name.endsWith(".png", ignoreCase = true) -> "image/png"
        else -> "application/json"
    }
}
