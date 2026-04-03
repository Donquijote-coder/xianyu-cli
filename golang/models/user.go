package models

// ParseProfile parses user profile API response into a flat map.
func ParseProfile(rawData map[string]interface{}) map[string]interface{} {
	user := rawData
	if u, ok := rawData["userInfo"].(map[string]interface{}); ok {
		user = u
	}

	return map[string]interface{}{
		"user_id":       getStr(user, "userId"),
		"nickname":      getStr(user, "nickName"),
		"avatar":        getStr(user, "avatarUrl"),
		"credit_score":  getStr(user, "creditLevel"),
		"item_count":    getNumeric(user, "itemCount"),
		"fans_count":    getNumeric(user, "fansCount"),
		"follow_count":  getNumeric(user, "followCount"),
		"location":      getStr(user, "area"),
		"register_time": getStr(user, "registerTime"),
	}
}
