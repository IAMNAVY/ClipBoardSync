package com.clipsync.android.network

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.util.concurrent.TimeUnit

@Serializable
data class LoginResponse(
    val message: String = "",
    val token: String = "",
    val refresh_token: String = "",
    val user_id: Int = 0,
    val username: String = "",
    val error: String = ""
)

object ApiClient {

    private val json = Json { ignoreUnknownKeys = true }
    private val jsonMediaType = "application/json; charset=utf-8".toMediaType()

    private val client = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(15, TimeUnit.SECONDS)
        .writeTimeout(15, TimeUnit.SECONDS)
        .build()

    suspend fun login(serverUrl: String, username: String, password: String): Result<LoginResponse> =
        withContext(Dispatchers.IO) {
            try {
                val body = """{"username":"$username","password":"$password"}"""
                    .toRequestBody(jsonMediaType)

                val request = Request.Builder()
                    .url("${serverUrl.trimEnd('/')}/api/login")
                    .post(body)
                    .build()

                val response = client.newCall(request).execute()
                val responseBody = response.body?.string() ?: ""
                val loginResp = json.decodeFromString<LoginResponse>(responseBody)

                if (response.isSuccessful && loginResp.token.isNotBlank()) {
                    Result.success(loginResp)
                } else {
                    val msg = loginResp.error.ifBlank { "HTTP ${response.code}" }
                    Result.failure(Exception("登录失败: $msg"))
                }
            } catch (e: Exception) {
                Result.failure(Exception("连接服务器失败: ${e.message}"))
            }
        }

    /**
     * Exchanges a refresh token for a new access+refresh token pair.
     */
    suspend fun refreshAccessToken(serverUrl: String, refreshToken: String): Result<LoginResponse> =
        withContext(Dispatchers.IO) {
            try {
                val body = """{"refresh_token":"$refreshToken"}"""
                    .toRequestBody(jsonMediaType)

                val request = Request.Builder()
                    .url("${serverUrl.trimEnd('/')}/api/refresh")
                    .post(body)
                    .build()

                val response = client.newCall(request).execute()
                val responseBody = response.body?.string() ?: ""
                val resp = json.decodeFromString<LoginResponse>(responseBody)

                if (response.isSuccessful && resp.token.isNotBlank()) {
                    Result.success(resp)
                } else {
                    val msg = resp.error.ifBlank { "HTTP ${response.code}" }
                    Result.failure(Exception("刷新 Token 失败: $msg"))
                }
            } catch (e: Exception) {
                Result.failure(Exception("刷新 Token 失败: ${e.message}"))
            }
        }

    suspend fun pushClipboard(
        serverUrl: String, token: String, content: String, deviceName: String
    ): Result<Unit> = withContext(Dispatchers.IO) {
        try {
            val jsonBody = kotlinx.serialization.json.buildJsonObject {
                put("content", kotlinx.serialization.json.JsonPrimitive(content))
                put("device_name", kotlinx.serialization.json.JsonPrimitive(deviceName))
            }.toString()

            val body = jsonBody.toRequestBody(jsonMediaType)

            val request = Request.Builder()
                .url("${serverUrl.trimEnd('/')}/api/clipboard")
                .addHeader("Authorization", "Bearer $token")
                .post(body)
                .build()

            val response = client.newCall(request).execute()
            if (response.isSuccessful) {
                Result.success(Unit)
            } else {
                Result.failure(Exception("上传失败: HTTP ${response.code}"))
            }
        } catch (e: Exception) {
            Result.failure(Exception("上传剪贴板失败: ${e.message}"))
        }
    }

    /**
     * Fetches clipboard history with optional search/filter/pinned params.
     */
    suspend fun getClipboardHistory(
        serverUrl: String, token: String,
        search: String = "", category: String = "", pinned: Boolean = false
    ): Result<ClipHistoryResponse> = withContext(Dispatchers.IO) {
        try {
            val params = mutableListOf<String>()
            if (search.isNotBlank()) params.add("search=${java.net.URLEncoder.encode(search, "UTF-8")}")
            if (pinned) params.add("pinned=true")
            else if (category.isNotBlank()) params.add("category=$category")
            val qs = if (params.isNotEmpty()) "?" + params.joinToString("&") else ""

            val request = Request.Builder()
                .url("${serverUrl.trimEnd('/')}/api/clipboard$qs")
                .addHeader("Authorization", "Bearer $token")
                .get()
                .build()

            val response = client.newCall(request).execute()
            val responseBody = response.body?.string() ?: ""

            if (response.isSuccessful) {
                val data = json.decodeFromString<ClipHistoryResponse>(responseBody)
                Result.success(data)
            } else {
                Result.failure(Exception("获取失败: HTTP ${response.code}"))
            }
        } catch (e: Exception) {
            Result.failure(Exception("获取剪贴板历史失败: ${e.message}"))
        }
    }

    /**
     * Toggles the pin/favorite status of a clipboard entry.
     */
    suspend fun togglePin(serverUrl: String, token: String, clipId: Int): Result<Unit> =
        withContext(Dispatchers.IO) {
            try {
                val request = Request.Builder()
                    .url("${serverUrl.trimEnd('/')}/api/clipboard/$clipId/pin")
                    .addHeader("Authorization", "Bearer $token")
                    .put("".toRequestBody(null))
                    .build()

                val response = client.newCall(request).execute()
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(Exception("操作失败: HTTP ${response.code}"))
                }
            } catch (e: Exception) {
                Result.failure(Exception("收藏操作失败: ${e.message}"))
            }
        }
}

@Serializable
data class ClipEntry(
    val id: Int = 0,
    val content: String = "",
    val category: String = "text",
    val is_pinned: Boolean = false,
    val device_name: String = "",
    val created_at: String = ""
)

@Serializable
data class ClipHistoryResponse(
    val entries: List<ClipEntry> = emptyList(),
    val count: Int = 0
)
