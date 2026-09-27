package dev.tavernagent.mobile.data.local

import android.content.Context
import android.net.Uri
import android.provider.OpenableColumns

/** 从 SAF URI 读取角色卡/剧情包字节，并按魔数推断媒体类型。 */
object FilePickers {

    data class PickedFile(val bytes: ByteArray, val mediaType: String, val name: String)

    fun read(context: Context, uri: Uri): PickedFile {
        val bytes = context.contentResolver.openInputStream(uri)?.use { it.readBytes() }
            ?: error("无法读取所选文件")
        val name = displayName(context, uri)
        return PickedFile(bytes = bytes, mediaType = sniffMediaType(bytes, name), name = name)
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
