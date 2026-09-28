package dev.tavernagent.mobile.data.local

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * 用 Android Keystore 中的 AES/GCM 密钥加密本地敏感值（Bearer Token、TOFU 证书）。
 *
 * 密钥不出 Keystore；即便 DataStore 文件被读取，也无法直接还原明文（配合 `allowBackup=false`
 * 阻断云/adb 备份外带）。Keystore 或算法不可用（如 JVM 单测）时回退为原样存储，
 * 保证不因加密失败而无法连接。
 */
object SecretCipher {
    private const val KEYSTORE = "AndroidKeyStore"
    private const val ALIAS = "tavern_secret_v1"
    private const val TRANSFORM = "AES/GCM/NoPadding"
    private const val IV_BYTES = 12
    private const val TAG_BITS = 128
    private const val PREFIX = "enc:v1:"

    private fun secretKey(): SecretKey? = runCatching {
        val keyStore = KeyStore.getInstance(KEYSTORE).apply { load(null) }
        (keyStore.getEntry(ALIAS, null) as? KeyStore.SecretKeyEntry)?.secretKey ?: generateKey()
    }.getOrNull()

    private fun generateKey(): SecretKey {
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, KEYSTORE)
        generator.init(
            KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .build(),
        )
        return generator.generateKey()
    }

    fun encrypt(plain: String): String {
        if (plain.isBlank()) return plain
        val key = secretKey() ?: return plain
        return runCatching {
            val cipher = Cipher.getInstance(TRANSFORM)
            cipher.init(Cipher.ENCRYPT_MODE, key)
            val cipherText = cipher.doFinal(plain.toByteArray(Charsets.UTF_8))
            PREFIX + Base64.encodeToString(cipher.iv + cipherText, Base64.NO_WRAP)
        }.getOrDefault(plain)
    }

    fun decrypt(stored: String): String {
        if (!stored.startsWith(PREFIX)) return stored
        val key = secretKey() ?: return ""
        return runCatching {
            val raw = Base64.decode(stored.removePrefix(PREFIX), Base64.NO_WRAP)
            val cipher = Cipher.getInstance(TRANSFORM)
            cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(TAG_BITS, raw.copyOfRange(0, IV_BYTES)))
            String(cipher.doFinal(raw.copyOfRange(IV_BYTES, raw.size)), Charsets.UTF_8)
        }.getOrDefault("")
    }
}
