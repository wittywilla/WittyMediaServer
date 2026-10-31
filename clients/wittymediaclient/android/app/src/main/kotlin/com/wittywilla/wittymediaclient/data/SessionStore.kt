package com.wittywilla.wittymediaclient.data

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import com.wittywilla.wittymediaclient.net.json
import kotlinx.serialization.Serializable
import kotlinx.serialization.encodeToString
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

// A server this device has used, with the identity key it proved.
@Serializable
data class SavedServer(
    val url: String,
    val name: String,
    val serverId: String = "",
    val publicKey: String = "",
    val fingerprint: String = "",
)

private const val MAX_RECENT = 8

// The most recent first, once per address and once per server.
fun rememberServer(recent: List<SavedServer>, server: SavedServer): List<SavedServer> {
    val others = recent.filter { it.url != server.url && (server.serverId.isEmpty() || it.serverId != server.serverId) }
    return (listOf(server) + others).take(MAX_RECENT)
}

// Which server this device uses and its sign-in. The token is encrypted with a
// key held by Android's Keystore, and app backups are off (see the manifest).
class SessionStore(context: Context) {
    private val prefs = context.getSharedPreferences("session", Context.MODE_PRIVATE)

    var server: String
        get() = prefs.getString(KEY_SERVER, "") ?: ""
        set(v) = prefs.edit().putString(KEY_SERVER, v).apply()

    var token: String
        get() = decrypt(prefs.getString(KEY_TOKEN, "") ?: "")
        set(v) = prefs.edit().putString(KEY_TOKEN, if (v.isEmpty()) "" else encrypt(v)).apply()

    var recent: List<SavedServer>
        get() = runCatching { json.decodeFromString<List<SavedServer>>(prefs.getString(KEY_RECENT, "[]") ?: "[]") }.getOrDefault(emptyList())
        set(v) = prefs.edit().putString(KEY_RECENT, json.encodeToString(v)).apply()

    fun remember(server: SavedServer) {
        recent = rememberServer(recent, server)
    }

    fun find(url: String): SavedServer? = recent.find { it.url == url }

    // The address, the key it proved and the token are kept together, in one write.
    fun signedIn(server: SavedServer, token: String) {
        prefs.edit()
            .putString(KEY_SERVER, server.url)
            .putString(KEY_RECENT, json.encodeToString(rememberServer(recent, server)))
            .putString(KEY_TOKEN, encrypt(token))
            .apply()
    }

    private fun key(): SecretKey {
        val store = KeyStore.getInstance(KEYSTORE).apply { load(null) }
        (store.getKey(ALIAS, null) as? SecretKey)?.let { return it }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, KEYSTORE)
        generator.init(
            KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .build(),
        )
        return generator.generateKey()
    }

    private fun encrypt(plain: String): String {
        val cipher = Cipher.getInstance(TRANSFORM).apply { init(Cipher.ENCRYPT_MODE, key()) }
        val sealed = cipher.iv + cipher.doFinal(plain.toByteArray())
        return Base64.encodeToString(sealed, Base64.NO_WRAP)
    }

    // A value that can't be read (the key was reset) means signing in again.
    private fun decrypt(stored: String): String {
        if (stored.isEmpty()) return ""
        return runCatching {
            val raw = Base64.decode(stored, Base64.NO_WRAP)
            val cipher = Cipher.getInstance(TRANSFORM)
            cipher.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, raw, 0, IV_BYTES))
            String(cipher.doFinal(raw, IV_BYTES, raw.size - IV_BYTES))
        }.getOrDefault("")
    }

    private companion object {
        const val KEY_SERVER = "server"
        const val KEY_TOKEN = "token"
        const val KEY_RECENT = "recent"
        const val KEYSTORE = "AndroidKeyStore"
        const val ALIAS = "wittymediaclient-session"
        const val TRANSFORM = "AES/GCM/NoPadding"
        const val IV_BYTES = 12
    }
}
