package com.clipsync.android.network

import android.util.Log
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicLong

class WsClient(
    private var serverUrl: String,
    private var token: String,
    private var deviceName: String,
    private val onClip: (String) -> Unit,
    private val onStatusChange: (Boolean) -> Unit,
    private val onDeviceRenamed: (String) -> Unit,
    private val onForceDisconnect: (String) -> Unit
) {
    companion object {
        private const val TAG = "WsClient"
    }

    private val json = Json { ignoreUnknownKeys = true }
    private val client = OkHttpClient.Builder()
        .pingInterval(30, TimeUnit.SECONDS)
        .readTimeout(0, TimeUnit.MILLISECONDS) // no timeout for WS
        .build()

    private var webSocket: WebSocket? = null
    private var clientId: AtomicLong = AtomicLong(0)
    private val isRunning = AtomicBoolean(false)
    private val forceDisconnected = AtomicBoolean(false)
    private var reconnectThread: Thread? = null

    fun start() {
        if (isRunning.getAndSet(true)) return
        forceDisconnected.set(false)
        connectWithRetry()
    }

    fun stop() {
        isRunning.set(false)
        reconnectThread?.interrupt()
        webSocket?.close(1000, "client stopping")
        webSocket = null
    }

    fun updateDeviceName(newName: String) {
        deviceName = newName
    }

    private fun buildWsUrl(): String {
        val base = serverUrl.trimEnd('/')
            .replace("https://", "wss://")
            .replace("http://", "ws://")
        return "$base/ws?token=$token&device_name=${java.net.URLEncoder.encode(deviceName, "UTF-8")}"
    }

    private fun connectWithRetry() {
        reconnectThread = Thread {
            var backoff = 1000L
            val maxBackoff = 30000L

            while (isRunning.get() && !forceDisconnected.get()) {
                try {
                    connect()
                    // If connect returns normally, the connection was closed
                    backoff = 1000L // reset on successful connection
                } catch (e: Exception) {
                    Log.e(TAG, "Connection error: ${e.message}")
                }

                if (!isRunning.get() || forceDisconnected.get()) break

                onStatusChange(false)
                try {
                    Thread.sleep(backoff)
                } catch (_: InterruptedException) {
                    break
                }
                backoff = (backoff * 2).coerceAtMost(maxBackoff)
            }
        }.also { it.start() }
    }

    private fun connect() {
        val url = buildWsUrl()
        Log.d(TAG, "Connecting to $url")

        val latch = java.util.concurrent.CountDownLatch(1)
        val request = Request.Builder().url(url).build()

        webSocket = client.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                Log.d(TAG, "Connected")
                onStatusChange(true)
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                handleMessage(text)
            }

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                webSocket.close(1000, null)
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                Log.d(TAG, "Closed: $code $reason")
                latch.countDown()
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                Log.e(TAG, "Failure: ${t.message}")
                latch.countDown()
            }
        })

        latch.await() // block until disconnected
        onStatusChange(false)
    }

    private fun handleMessage(text: String) {
        try {
            val msg = json.decodeFromString<JsonObject>(text)
            val type = msg["type"]?.jsonPrimitive?.contentOrNull ?: return

            when (type) {
                "clip" -> {
                    val content = msg["content"]?.jsonPrimitive?.contentOrNull
                    if (!content.isNullOrBlank()) {
                        Log.d(TAG, "Received clip (${content.length} chars)")
                        onClip(content)
                    }
                }

                "welcome" -> {
                    val id = msg["client_id"]?.jsonPrimitive?.doubleOrNull?.toLong() ?: 0
                    clientId.set(id)
                    Log.d(TAG, "Welcome, client_id=$id")
                }

                "devices_update" -> {
                    handleDevicesUpdate(msg)
                }

                "force_disconnect" -> {
                    val reason = msg["reason"]?.jsonPrimitive?.contentOrNull ?: "unknown"
                    Log.w(TAG, "Force disconnected: $reason")
                    forceDisconnected.set(true)
                    onForceDisconnect(reason)
                }
            }
        } catch (e: Exception) {
            Log.e(TAG, "Parse error: ${e.message}")
        }
    }

    private fun handleDevicesUpdate(msg: JsonObject) {
        val myId = clientId.get()
        if (myId == 0L) return

        val devices = msg["devices"]?.jsonArray ?: return
        for (device in devices) {
            val dev = device.jsonObject
            val id = dev["id"]?.jsonPrimitive?.doubleOrNull?.toLong() ?: continue
            if (id == myId) {
                val serverName = dev["device_name"]?.jsonPrimitive?.contentOrNull ?: break
                if (serverName.isNotBlank() && serverName != deviceName) {
                    Log.d(TAG, "Server renamed device: '$deviceName' -> '$serverName'")
                    deviceName = serverName
                    onDeviceRenamed(serverName)
                }
                break
            }
        }
    }
}
