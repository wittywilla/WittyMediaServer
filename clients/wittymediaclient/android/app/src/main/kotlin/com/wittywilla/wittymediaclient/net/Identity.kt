package com.wittywilla.wittymediaclient.net

import com.wittywilla.wittymediaclient.data.SavedServer
import kotlinx.serialization.Serializable
import org.bouncycastle.math.ec.rfc8032.Ed25519
import java.security.MessageDigest
import java.security.SecureRandom
import kotlin.io.encoding.Base64

// Checks that an address is answered by the server we think it is: it has to
// sign a fresh random value with its Ed25519 key. See docs/CLIENT-API.md, "Server identity".

@Serializable
data class IdentityRequest(val nonce: String)

@Serializable
data class IdentityAnswer(
    val serverId: String = "",
    val publicKey: String = "",
    val fingerprint: String = "",
    val algorithm: String = "",
    val signature: String = "",
)

// Unreachable  nothing answered
// None         a server older than this check, and no key was remembered for it
// Bad          the answer proves nothing, so nothing about it can be believed
// New          never seen before
// Same         a key we remembered
// Different    we remembered another key here: possibly a look-alike
enum class IdentityState { Unreachable, None, Bad, New, Same, Different }

data class ServerProof(
    val state: IdentityState,
    val serverId: String = "",
    val publicKey: String = "",
    val fingerprint: String = "",
) {
    fun savedAs(url: String, name: String) = SavedServer(url, name, serverId, publicKey, fingerprint)
}

enum class NextStep { Warn, Refuse, Lost, AskCode, SignIn, Open }

object Identity {
    private const val NONCE_BYTES = 32
    private const val KEY_BYTES = 32
    private const val SIGNATURE_BYTES = 64
    private val random by lazy { SecureRandom() }

    fun newNonce(): String = Base64.encode(ByteArray(NONCE_BYTES).also { random.nextBytes(it) })

    private fun signedBytes(serverId: String, nonce: String) =
        "WittyMediaServer identity v1\n$serverId\n$nonce".toByteArray(Charsets.UTF_8)

    // Is this answer really signed by the key it names?
    fun signatureValid(answer: IdentityAnswer, nonce: String): Boolean {
        if (answer.algorithm != "ed25519" || answer.serverId.isEmpty()) return false
        return try {
            val key = Base64.decode(answer.publicKey)
            val signature = Base64.decode(answer.signature)
            if (key.size != KEY_BYTES || signature.size != SIGNATURE_BYTES) return false
            val message = signedBytes(answer.serverId, nonce)
            Ed25519.verify(signature, 0, key, 0, message, 0, message.size)
        } catch (_: IllegalArgumentException) {
            false // not base64
        }
    }

    // A short form of the key for people to compare by eye. Worked out here, since
    // a look-alike could claim another server's fingerprint in its answer.
    fun fingerprint(publicKey: String): String {
        val sum = MessageDigest.getInstance("SHA-256").digest(Base64.decode(publicKey))
        val hex = sum.take(6).joinToString("") { "%02X".format(it) }
        return "${hex.substring(0, 4)}-${hex.substring(4, 8)}-${hex.substring(8)}"
    }

    // What a good signature means, given the saved servers. Holding a remembered key
    // proves who it is, wherever it answers. Otherwise a remembered address or ID
    // answering with another key is what a look-alike would look like.
    fun judge(answer: IdentityAnswer, nonce: String, url: String, saved: List<SavedServer>): IdentityState {
        if (!signatureValid(answer, nonce)) return IdentityState.Bad
        val pinned = saved.filter { it.publicKey.isNotEmpty() }
        return when {
            pinned.any { it.publicKey == answer.publicKey } -> IdentityState.Same
            pinned.any { it.url == url || it.serverId == answer.serverId } -> IdentityState.Different
            else -> IdentityState.New
        }
    }

    // A 404 is an older server, unless we remember this address proving itself:
    // then it's what a look-alike would say to avoid the question.
    fun unanswered(url: String, saved: List<SavedServer>) =
        if (saved.any { it.url == url && it.publicKey.isNotEmpty() }) IdentityState.Bad else IdentityState.None

    // May what belongs with this pin (a token, a code) go to the server that gave this proof?
    fun fits(pin: String, proof: ServerProof): Boolean =
        if (pin.isEmpty()) proof.state == IdentityState.New || proof.state == IdentityState.None
        else proof.state == IdentityState.Same && proof.publicKey == pin

    // After someone chooses a server.
    fun nextStep(state: IdentityState, tokenFits: Boolean) = when (state) {
        IdentityState.Different -> NextStep.Warn
        IdentityState.Bad -> NextStep.Refuse
        IdentityState.Unreachable -> NextStep.Lost
        IdentityState.New -> NextStep.AskCode
        IdentityState.None -> NextStep.SignIn
        IdentityState.Same -> if (tokenFits) NextStep.Open else NextStep.SignIn
    }

    // When the app starts, with the server it was last using.
    fun stepAtStart(state: IdentityState, tokenFits: Boolean) = when (state) {
        IdentityState.Different -> NextStep.Warn
        IdentityState.Bad -> NextStep.Refuse
        IdentityState.Unreachable -> NextStep.Lost
        IdentityState.New, IdentityState.None, IdentityState.Same -> if (tokenFits) NextStep.Open else NextStep.SignIn
    }
}
