package main

import (
	"log"
	"net/http"
	"regexp"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ============================================================================
// CleanRule CRUD handlers & content filtering engine
// ============================================================================

// handleGetCleanRules returns all clean rules for the authenticated user.
func handleGetCleanRules(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	var rules []CleanRule
	db.Where("user_id = ?", userID).Order("id ASC").Find(&rules)
	c.JSON(http.StatusOK, gin.H{"rules": rules, "count": len(rules)})
}

// handleCreateCleanRule creates a new clean rule.
func handleCreateCleanRule(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	var req CleanRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate regex pattern
	if _, err := regexp.Compile(req.Pattern); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid regex pattern: " + err.Error()})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	rule := CleanRule{
		UserID:      userID,
		Pattern:     req.Pattern,
		Action:      req.Action,
		Description: req.Description,
		Enabled:     enabled,
	}
	if err := db.Create(&rule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create rule"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "rule created", "rule": rule})
}

// handleUpdateCleanRule updates an existing clean rule.
func handleUpdateCleanRule(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule id"})
		return
	}

	var rule CleanRule
	if db.Where("id = ? AND user_id = ?", id, userID).First(&rule).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}

	var req CleanRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate regex pattern
	if _, err := regexp.Compile(req.Pattern); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid regex pattern: " + err.Error()})
		return
	}

	rule.Pattern = req.Pattern
	rule.Action = req.Action
	rule.Description = req.Description
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	db.Save(&rule)
	c.JSON(http.StatusOK, gin.H{"message": "rule updated", "rule": rule})
}

// handleDeleteCleanRule deletes a clean rule.
func handleDeleteCleanRule(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)
	id := c.Param("id")

	result := db.Where("id = ? AND user_id = ?", id, userID).Delete(&CleanRule{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "rule deleted"})
}

// ============================================================================
// Content filtering engine
// ============================================================================

// applyCleanRules applies all enabled clean rules for a user to the content.
// Returns (processedContent, shouldIgnore).
// If shouldIgnore is true, the content should not be saved or broadcast at all.
func applyCleanRules(userID uint, content string) (string, bool) {
	var rules []CleanRule
	db.Where("user_id = ? AND enabled = ?", userID, true).Find(&rules)

	if len(rules) == 0 {
		return content, false
	}

	result := content
	for _, rule := range rules {
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			log.Printf("[cleanrule] 正则编译失败 (rule %d): %v", rule.ID, err)
			continue
		}

		switch rule.Action {
		case "ignore":
			// If the pattern matches anywhere in the content, ignore the entire entry
			if re.MatchString(result) {
				log.Printf("[cleanrule] 内容匹配忽略规则 %d (%s)，跳过同步", rule.ID, rule.Description)
				return "", true
			}
		case "clean":
			// Strip the matched portions from the content
			cleaned := re.ReplaceAllString(result, "")
			if cleaned != result {
				log.Printf("[cleanrule] 内容被清理规则 %d (%s) 处理", rule.ID, rule.Description)
				result = cleaned
			}
		}
	}

	return result, false
}
