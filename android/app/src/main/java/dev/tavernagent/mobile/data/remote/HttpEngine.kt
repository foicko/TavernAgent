package dev.tavernagent.mobile.data.remote

import java.net.InetSocketAddress
import java.security.KeyStore
import java.security.MessageDigest
import java.security.SecureRandom
import java.security.cert.CertificateException
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLSocket
import javax.net.ssl.SSLSocketFactory
import javax.net.ssl.TrustManagerFactory
import javax.net.ssl.X509TrustManager
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * 传输层辅助。
 *
 * HTTPS 采用 TOFU（首次使用即信任）：用户确认指纹后只信任该证书本身，
 * 而不是全局跳过校验，这样 `-tls auto` 的自签证书也能安全使用。
 */
object HttpEngine {

    /** OkHttp 需要成对的 socketFactory + trustManager。 */
    data class TrustedTls(val socketFactory: SSLSocketFactory, val trustManager: X509TrustManager)

    fun tlsFor(der: ByteArray): TrustedTls {
        val certificate = CertificateFactory.getInstance("X.509")
            .generateCertificate(der.inputStream()) as X509Certificate
        val keyStore = KeyStore.getInstance(KeyStore.getDefaultType()).apply {
            load(null)
            setCertificateEntry("tavern-server", certificate)
        }
        val factory = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm())
        factory.init(keyStore)
        val manager = factory.trustManagers.filterIsInstance<X509TrustManager>().first()
        val context = SSLContext.getInstance("TLS")
        context.init(null, arrayOf<X509TrustManager>(manager), null)
        return TrustedTls(context.socketFactory, manager)
    }

    /** 用未校验的握手读回对端证书，用于让用户确认指纹。 */
    suspend fun fetchPeerCertificate(host: String, port: Int, timeoutMillis: Int = 8000): X509Certificate =
        // 必须在 IO 线程执行：这是阻塞 socket（连接 + 握手最长 8s），而调用方
        // （ConnectViewModel）跑在 Main.immediate，直接调用会触发
        // NetworkOnMainThreadException 或 ANR。
        withContext(Dispatchers.IO) {
            val context = SSLContext.getInstance("TLS")
            context.init(null, arrayOf<javax.net.ssl.TrustManager>(permissiveManager()), SecureRandom())
            val socket = context.socketFactory.createSocket() as SSLSocket
            try {
                socket.connect(InetSocketAddress(host, port), timeoutMillis)
                socket.startHandshake()
                socket.session.peerCertificates.first() as X509Certificate
            } finally {
                runCatching { socket.close() }
            }
        }

    fun sha256Fingerprint(certificate: X509Certificate): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(certificate.encoded)
        return digest.joinToString(":") { "%02X".format(it) }
    }

    fun toDer(certificate: X509Certificate): ByteArray = certificate.encoded

    private fun permissiveManager(): X509TrustManager = object : X509TrustManager {
        override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) = Unit
        override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {
            if (chain.isNullOrEmpty()) throw CertificateException("服务器未提供证书")
        }

        override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
    }
}
