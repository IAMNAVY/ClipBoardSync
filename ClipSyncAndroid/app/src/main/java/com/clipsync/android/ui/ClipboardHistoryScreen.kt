package com.clipsync.android.ui

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.widget.Toast
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.filled.StarBorder
import androidx.compose.material3.AssistChip
import androidx.compose.material3.AssistChipDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.clipsync.android.data.AppConfig
import com.clipsync.android.network.ApiClient
import com.clipsync.android.network.ClipEntry
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

data class FilterOption(val label: String, val category: String, val isPinned: Boolean = false)

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun ClipboardHistoryScreen(config: AppConfig) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val entries = remember { mutableStateListOf<ClipEntry>() }
    var isLoading by remember { mutableStateOf(false) }
    var searchText by remember { mutableStateOf("") }
    var selectedFilter by remember { mutableStateOf("") }
    var isPinnedFilter by remember { mutableStateOf(false) }
    var searchJob by remember { mutableStateOf<Job?>(null) }

    val filters = listOf(
        FilterOption("全部", ""),
        FilterOption("📝 文本", "text"),
        FilterOption("🔗 链接", "url"),
        FilterOption("💻 代码", "code"),
        FilterOption("⭐ 收藏", "", isPinned = true)
    )

    fun loadData() {
        scope.launch {
            isLoading = true
            val result = ApiClient.getClipboardHistory(
                config.serverUrl, config.token,
                search = searchText,
                category = if (isPinnedFilter) "" else selectedFilter,
                pinned = isPinnedFilter
            )
            result.fold(
                onSuccess = { resp ->
                    entries.clear()
                    entries.addAll(resp.entries)
                },
                onFailure = {
                    Toast.makeText(context, "加载失败: ${it.message}", Toast.LENGTH_SHORT).show()
                }
            )
            isLoading = false
        }
    }

    // Initial load
    LaunchedEffect(Unit) { loadData() }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(horizontal = 16.dp, vertical = 8.dp)
    ) {
        // Search and Filter Card (Matches Settings Page Style)
        Card(
            modifier = Modifier.fillMaxWidth().padding(bottom = 16.dp),
            colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)
        ) {
            Column(modifier = Modifier.padding(16.dp)) {
                OutlinedTextField(
                    value = searchText,
                    onValueChange = { text ->
                        searchText = text
                        searchJob?.cancel()
                        searchJob = scope.launch {
                            delay(400)
                            loadData()
                        }
                    },
                    placeholder = { Text("搜索剪贴板内容...", fontSize = 14.sp) },
                    leadingIcon = { Icon(Icons.Default.Search, contentDescription = null, modifier = Modifier.size(20.dp)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    shape = androidx.compose.foundation.shape.RoundedCornerShape(12.dp),
                    colors = androidx.compose.material3.OutlinedTextFieldDefaults.colors(
                        focusedBorderColor = MaterialTheme.colorScheme.primary,
                        unfocusedBorderColor = MaterialTheme.colorScheme.outlineVariant
                    )
                )

                Spacer(modifier = Modifier.height(12.dp))

                FlowRow(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    filters.forEach { filter ->
                        val isSelected = if (filter.isPinned) isPinnedFilter
                            else (!isPinnedFilter && selectedFilter == filter.category)

                        androidx.compose.material3.FilterChip(
                            selected = isSelected,
                            onClick = {
                                if (filter.isPinned) {
                                    isPinnedFilter = !isPinnedFilter
                                    if (isPinnedFilter) selectedFilter = ""
                                } else {
                                    isPinnedFilter = false
                                    selectedFilter = filter.category
                                }
                                loadData()
                            },
                            label = { 
                                Text(
                                    filter.label, 
                                    fontSize = 13.sp, 
                                    fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal
                                ) 
                            },
                            colors = androidx.compose.material3.FilterChipDefaults.filterChipColors(
                                selectedContainerColor = MaterialTheme.colorScheme.primary,
                                selectedLabelColor = MaterialTheme.colorScheme.onPrimary
                            ),
                            shape = androidx.compose.foundation.shape.RoundedCornerShape(8.dp)
                        )
                    }
                }
            }
        }

        // Loading indicator or list
        if (isLoading && entries.isEmpty()) {
            Column(
                modifier = Modifier.fillMaxSize(),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.Center
            ) {
                CircularProgressIndicator()
                Spacer(modifier = Modifier.height(8.dp))
                Text("加载中...", color = MaterialTheme.colorScheme.onSurface.copy(0.5f))
            }
        } else if (entries.isEmpty()) {
            Column(
                modifier = Modifier.fillMaxSize(),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.Center
            ) {
                Text("暂无记录", color = MaterialTheme.colorScheme.onSurface.copy(0.5f), fontSize = 16.sp)
            }
        } else {
            LazyColumn(modifier = Modifier.fillMaxSize()) {
                items(entries, key = { it.id }) { entry ->
                    ClipItemRow(
                        entry = entry,
                        onCopy = {
                            val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                            clipboard.setPrimaryClip(ClipData.newPlainText("ClipSync", entry.content))
                            Toast.makeText(context, "已复制", Toast.LENGTH_SHORT).show()
                        },
                        onTogglePin = {
                            scope.launch {
                                ApiClient.togglePin(config.serverUrl, config.token, entry.id)
                                loadData()
                            }
                        }
                    )
                    HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.3f))
                }
            }
        }
    }
}

@Composable
private fun ClipItemRow(
    entry: ClipEntry,
    onCopy: () -> Unit,
    onTogglePin: () -> Unit
) {
    val catEmoji = when (entry.category) {
        "url" -> "🔗"
        "code" -> "💻"
        else -> "📝"
    }

    // Parse time
    val timeDisplay = try {
        val parts = entry.created_at.split("T")
        if (parts.size >= 2) {
            val datePart = parts[0].substring(5) // MM-DD
            val timePart = parts[1].substring(0, 5) // HH:MM
            "$datePart $timePart"
        } else entry.created_at
    } catch (_: Exception) { entry.created_at }

    Card(
        modifier = Modifier
            .fillMaxWidth()
            .clickable { onCopy() }
            .padding(vertical = 4.dp),
        colors = CardDefaults.cardColors(
            containerColor = if (entry.is_pinned)
                MaterialTheme.colorScheme.primary.copy(alpha = 0.06f)
            else MaterialTheme.colorScheme.surface
        )
    ) {
        Column(modifier = Modifier.padding(12.dp)) {
            // Content preview
            Text(
                text = entry.content,
                maxLines = 4,
                overflow = TextOverflow.Ellipsis,
                fontSize = 14.sp,
                lineHeight = 20.sp
            )

            Spacer(modifier = Modifier.height(6.dp))

            // Bottom row: category + time + device | actions
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(catEmoji, fontSize = 12.sp)
                Spacer(modifier = Modifier.width(4.dp))
                Text(
                    text = timeDisplay + if (entry.device_name.isNotBlank()) " · ${entry.device_name}" else "",
                    fontSize = 11.sp,
                    color = MaterialTheme.colorScheme.onSurface.copy(0.5f),
                    modifier = Modifier.weight(1f)
                )

                IconButton(onClick = onTogglePin, modifier = Modifier.size(32.dp)) {
                    Icon(
                        if (entry.is_pinned) Icons.Default.Star else Icons.Default.StarBorder,
                        contentDescription = if (entry.is_pinned) "取消收藏" else "收藏",
                        tint = if (entry.is_pinned) MaterialTheme.colorScheme.primary
                            else MaterialTheme.colorScheme.onSurface.copy(0.4f),
                        modifier = Modifier.size(18.dp)
                    )
                }
                IconButton(onClick = onCopy, modifier = Modifier.size(32.dp)) {
                    Icon(
                        Icons.Default.ContentCopy,
                        contentDescription = "复制",
                        tint = MaterialTheme.colorScheme.onSurface.copy(0.4f),
                        modifier = Modifier.size(18.dp)
                    )
                }
            }
        }
    }
}
