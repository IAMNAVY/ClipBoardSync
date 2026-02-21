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
}
