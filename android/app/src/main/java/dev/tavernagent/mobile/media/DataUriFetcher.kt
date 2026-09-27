package dev.tavernagent.mobile.media

import android.net.Uri
import android.util.Base64
import coil.ImageLoader
import coil.decode.DataSource
import coil.decode.ImageSource
import coil.fetch.FetchResult
import coil.fetch.Fetcher
import coil.fetch.SourceResult
import coil.request.Options
import okio.Buffer

/**
 * Coil 自定义 Fetcher：支持从 Data URI（如 `data:image/jpeg;base64,...`）解码图像。
 *
 * 解决原生 TavernAgent 角色卡头像与内嵌缩略图以 Base64 Data URL 格式存储时，
 * Coil 默认因无对应 Fetcher 而抛出异常并降级为空白/占位符的问题。
 */
class DataUriFetcher(
    private val dataUriString: String,
    private val options: Options,
) : Fetcher {

    override suspend fun fetch(): FetchResult {
        val commaIndex = dataUriString.indexOf(',')
        require(commaIndex != -1) { "Invalid Data URI format" }

        val metadata = dataUriString.substring(0, commaIndex)
        val rawData = dataUriString.substring(commaIndex + 1)
        val isBase64 = metadata.contains(";base64", ignoreCase = true)
        val mimeType = metadata.removePrefix("data:").substringBefore(';').trim().ifBlank { "image/jpeg" }

        val bytes = if (isBase64) {
            Base64.decode(rawData, Base64.DEFAULT)
        } else {
            Uri.decode(rawData).toByteArray(Charsets.UTF_8)
        }

        val buffer = Buffer().write(bytes)
        return SourceResult(
            source = ImageSource(source = buffer, context = options.context),
            mimeType = mimeType,
            dataSource = DataSource.MEMORY,
        )
    }

    class StringFactory : Fetcher.Factory<String> {
        override fun create(data: String, options: Options, imageLoader: ImageLoader): Fetcher? {
            if (!data.startsWith("data:", ignoreCase = true)) return null
            return DataUriFetcher(data, options)
        }
    }

    class UriFactory : Fetcher.Factory<Uri> {
        override fun create(data: Uri, options: Options, imageLoader: ImageLoader): Fetcher? {
            if (!data.scheme.equals("data", ignoreCase = true)) return null
            return DataUriFetcher(data.toString(), options)
        }
    }
}
