package main

import (
	"net/url"
	"strings"
)

// ============================================================================
// URL tracking parameter stripper
// ============================================================================

// Common tracking parameters found across many platforms.
var globalTrackingParams = map[string]bool{
	// UTM family
	"utm_source": true, "utm_medium": true, "utm_campaign": true,
	"utm_term": true, "utm_content": true, "utm_id": true,
	"utm_cid": true, "utm_reader": true, "utm_name": true,

	// Chinese platform common params
	"spm":             true,
	"spm_id_from":     true,
	"from":            true,
	"from_source":     true,
	"share_source":    true,
	"share_medium":    true,
	"share_plat":      true,
	"share_session_id": true,
	"share_tag":       true,
	"share_times":     true,
	"timestamp":       true,
	"unique_k":        true,
	"vd_source":       true,
	"buvid":           true,
	"mid":             true,
	"seid":            true,
	"track_id":        true,
	"bbid":            true,
	"ts":              true,
	"from_spmid":      true,
	"is_story_h5":     true,
	"share_from":      true,

	// Taobao / Alibaba
	"scm":             true,
	"pvid":            true,
	"algo_pvid":       true,
	"algo_expid":      true,
	"bxsign":          true,
	"sck":             true,

	// Zhihu
	"search_source":   true,

	// Xiaohongshu
	"xhsshare": true,
	"appuid":   true,
	"apptime":  true,

	// Douyin / TikTok
	"sec_uid":         true,
	"previous_page":   true,
	"enter_from":      true,

	// Weibo
	"wm":   true,
	"nick": true,

	// Generic tracking
	"ref":       true,
	"referer":   true,
	"referrer":  true,
	"source":    true,
	"channel":   true,
	"subchannel": true,
	"ck":        true,
	"clickid":   true,
	"click_id":  true,
	"fbclid":    true,
	"gclid":     true,
	"dclid":     true,
	"msclkid":   true,
	"_t":        true,
}

// CleanTrackingURL strips tracking query parameters from URLs.
// Only processes content that looks like a single-line URL.
// Returns the original content unchanged if it's not a URL.
func CleanTrackingURL(content string) string {
	trimmed := strings.TrimSpace(content)

	// Only process single-line content that looks like a URL
	if strings.Contains(trimmed, "\n") {
		return content
	}
	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		return content
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return content
	}

	// No query params, nothing to clean
	if parsed.RawQuery == "" {
		return content
	}

	query := parsed.Query()
	cleaned := url.Values{}
	changed := false

	for key, values := range query {
		keyLower := strings.ToLower(key)

		// Check if it's a known tracking parameter
		if globalTrackingParams[keyLower] {
			changed = true
			continue
		}

		// Check for utm_* wildcard pattern
		if strings.HasPrefix(keyLower, "utm_") {
			changed = true
			continue
		}

		// Check for share_* wildcard pattern
		if strings.HasPrefix(keyLower, "share_") {
			changed = true
			continue
		}

		// Keep this parameter
		cleaned[key] = values
	}

	if !changed {
		return content
	}

	parsed.RawQuery = cleaned.Encode()
	result := parsed.String()

	// If all query params were removed, strip the trailing ?
	if parsed.RawQuery == "" {
		result = strings.TrimRight(result, "?")
	}

	return result
}
