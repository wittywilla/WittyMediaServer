package com.wittywilla.wittymediaclient

import com.wittywilla.wittymediaclient.net.Api
import com.wittywilla.wittymediaclient.net.ApiError
import com.wittywilla.wittymediaclient.net.IdentityState
import com.wittywilla.wittymediaclient.net.Probe
import com.wittywilla.wittymediaclient.net.identify
import com.wittywilla.wittymediaclient.net.probeServer
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Assume.assumeTrue
import org.junit.Test

// The whole typed-code path against a real server, when one is given. Skipped otherwise.
// WMS_LIVE_URL is its address. WMS_LIVE_CODE is a fresh number from POST /pair/invite.
class LiveServerTest {
    private val url = System.getenv("WMS_LIVE_URL").orEmpty()
    private val code = System.getenv("WMS_LIVE_CODE").orEmpty()

    @Test
    fun connectsLikeTheApp() = runBlocking {
        assumeTrue(url.isNotEmpty() && code.isNotEmpty())

        assertTrue(probeServer(url) is Probe.Found)

        val first = identify(url, emptyList())
        assertEquals(IdentityState.New, first.state)
        assertTrue(first.fingerprint.isNotEmpty())
        val pinned = first.savedAs(url, "Live")

        assertEquals(IdentityState.Same, identify(url, listOf(pinned)).state)
        val lookAlike = pinned.copy(publicKey = "ETNweKcsBs/GVCsvdOUSvznsUL93EKstZelyfa67ltA=")
        assertEquals(IdentityState.Different, identify(url, listOf(lookAlike)).state)

        val api = Api(url, "", quick = true)
        val wrong = (code.toInt() + 1).mod(1_000_000).toString().padStart(6, '0')
        try {
            api.pairRedeem(wrong, "Live test", "Android TV")
            fail("a wrong code should be refused")
        } catch (e: ApiError) {
            assertEquals(400, e.status)
            assertTrue(e.message.orEmpty().isNotEmpty())
        }

        val signedIn = api.pairRedeem(code, "Live test", "Android TV")
        assertTrue(signedIn.token.isNotEmpty())
        assertNotNull(Api(url, signedIn.token).status().user)

        try {
            api.pairRedeem(code, "Live test", "Android TV")
            fail("a used code should be refused")
        } catch (e: ApiError) {
            assertEquals(410, e.status)
        }

        Api(url, signedIn.token).logout()
    }
}
