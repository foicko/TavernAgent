package dev.tavernagent.mobile.data.repo

import dev.tavernagent.mobile.data.remote.ApiProvider
import dev.tavernagent.mobile.data.remote.dto.ConfigCatalogDto
import dev.tavernagent.mobile.data.remote.dto.ModelInstanceDto
import dev.tavernagent.mobile.data.remote.dto.ProbeResultDto
import dev.tavernagent.mobile.data.remote.dto.SlotBindingDto
import dev.tavernagent.mobile.data.remote.dto.VoiceDto
import kotlinx.coroutines.flow.emitAll

/** 模型连接与三槽位（primary 对话 / assist 导演 / reflection 记忆）。 */
class ModelsRepository(private val apiProvider: ApiProvider) {

    suspend fun catalog(): ConfigCatalogDto = apiProvider.current().models.catalog()

    suspend fun save(instance: ModelInstanceDto): ModelInstanceDto =
        apiProvider.current().models.saveModel(instance)

    suspend fun delete(modelId: String) = apiProvider.current().models.deleteModel(modelId)

    suspend fun probeModel(modelId: String, format: Boolean = false): ProbeResultDto =
        apiProvider.current().models.probeModel(modelId, format)

    suspend fun slots(): List<SlotBindingDto> = apiProvider.current().models.providers()

    suspend fun saveSlot(slot: String, enabled: Boolean, modelId: String): List<SlotBindingDto> =
        apiProvider.current().models.saveSlot(slot, enabled, modelId)

    suspend fun probeSlot(slot: String, format: Boolean = false): ProbeResultDto =
        apiProvider.current().models.probeProvider(slot, format)
}

/** 语音与图像（凭据由客户端直传后端，仅存本机）。 */
class MediaRepository(private val apiProvider: ApiProvider) {

    suspend fun voices(): List<VoiceDto> = apiProvider.current().media.voices()

    suspend fun synthesize(
        voice: String,
        text: String,
        instruction: String? = null,
        engine: String? = null,
        baseUrl: String? = null,
        apiKey: String? = null,
        model: String? = null,
        speed: Double? = null,
    ): ByteArray = apiProvider.current().media.synthesize(
        dev.tavernagent.mobile.data.remote.dto.TtsRequestDto(
            voice = voice,
            text = text,
            instruction = instruction,
            engine = engine,
            baseUrl = baseUrl,
            apiKey = apiKey,
            model = model,
            speed = speed,
        ),
    )

    suspend fun generateImage(
        prompt: String,
        negativePrompt: String? = null,
        size: String? = null,
        engine: String? = null,
        baseUrl: String? = null,
        apiKey: String? = null,
        model: String? = null,
    ): dev.tavernagent.mobile.data.remote.dto.GeneratedImageDto =
        apiProvider.current().media.generateImage(
            dev.tavernagent.mobile.data.remote.dto.ImageGenerateOptionsDto(
                engine = engine,
                prompt = prompt,
                negativePrompt = negativePrompt,
                size = size,
                baseUrl = baseUrl,
                apiKey = apiKey,
                model = model,
            ),
        )

    suspend fun absoluteUrl(path: String): String = apiProvider.current().media.absoluteUrl(path)
}
