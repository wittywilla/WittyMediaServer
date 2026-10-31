package com.wittywilla.wittymediaclient

import android.app.Application
import android.app.UiModeManager
import android.content.res.Configuration
import android.os.Build
import coil3.ImageLoader
import coil3.PlatformContext
import coil3.SingletonImageLoader
import coil3.disk.DiskCache
import coil3.memory.MemoryCache
import coil3.network.okhttp.OkHttpNetworkFetcherFactory
import coil3.request.crossfade
import com.wittywilla.wittymediaclient.data.SessionStore
import com.wittywilla.wittymediaclient.net.Api
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import okio.Path.Companion.toOkioPath

class WittyApp : Application(), SingletonImageLoader.Factory {
    val store by lazy { SessionStore(this) }

    // For work that must outlive the screen that started it.
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)

    // The current connection. Images use its client, so they carry the sign-in.
    @Volatile
    var api: Api = Api("", "")

    val isTv: Boolean by lazy {
        (getSystemService(UI_MODE_SERVICE) as UiModeManager).currentModeType == Configuration.UI_MODE_TYPE_TELEVISION
    }

    // How this device is named in the server's list of sign-ins.
    val deviceName: String get() = "${Build.MANUFACTURER} ${Build.MODEL}".take(60)
    val platformName: String get() = if (isTv) "Android TV" else "Android"

    // Small bounded caches: a TV stick has little memory to spare.
    override fun newImageLoader(context: PlatformContext): ImageLoader =
        ImageLoader.Builder(context)
            .components { add(OkHttpNetworkFetcherFactory(callFactory = { api.http })) }
            .memoryCache { MemoryCache.Builder().maxSizePercent(context, 0.15).build() }
            .diskCache {
                DiskCache.Builder()
                    .directory(cacheDir.resolve("images").toOkioPath())
                    .maxSizeBytes(100L * 1024 * 1024)
                    .build()
            }
            .crossfade(false)
            .build()
}
