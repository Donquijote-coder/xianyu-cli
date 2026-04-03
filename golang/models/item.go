package models

import (
	"fmt"
	"strings"
)

// ParseSearchItems parses search API response into a flat list of item maps.
func ParseSearchItems(rawData map[string]interface{}) []map[string]interface{} {
	var items []map[string]interface{}

	resultList, _ := rawData["resultList"].([]interface{})
	for _, entry := range resultList {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}

		dataMap := getNestedMap(entryMap, "data")
		itemMap := getNestedMap(dataMap, "item")
		main := getNestedMap(itemMap, "main")
		if main == nil {
			continue
		}

		ex := getNestedMap(main, "exContent")
		detail := getNestedMap(ex, "detailParams")
		args := getNestedMap(getNestedMap(main, "clickParam"), "args")

		itemID := getStr(detail, "itemId")
		if itemID == "" {
			itemID = getStr(ex, "itemId")
		}
		if itemID == "" {
			itemID = getStr(args, "id")
		}

		title := getStr(detail, "title")
		if title == "" {
			title = getStr(ex, "title")
		}

		rawPriceCents := getStr(detail, "soldPrice")
		if rawPriceCents == "" {
			rawPriceCents = getStr(args, "price")
		}
		priceFromEx := ""
		if rawPriceCents == "" {
			exPrice := ex["price"]
			switch p := exPrice.(type) {
			case []interface{}:
				var parts []string
				for _, elem := range p {
					if m, ok := elem.(map[string]interface{}); ok {
						t := getStr(m, "type")
						if t == "integer" || t == "decimal" {
							parts = append(parts, getStr(m, "text"))
						}
					}
				}
				priceFromEx = strings.Join(parts, "")
			case string:
				priceFromEx = p
			}
		}

		sellerName := getStr(detail, "userNick")
		if sellerName == "" {
			sellerName = getStr(ex, "userNickName")
		}

		location := getStr(ex, "area")
		if location == "" {
			location = getStr(args, "p_city")
		}

		// Price: soldPrice and args.price are in yuan (元), not cents.
		// exContent.price structured array also provides yuan values.
		var price string
		if rawPriceCents != "" {
			price = rawPriceCents
		} else if priceFromEx != "" {
			price = priceFromEx
		}

		sellerCredit := getStr(detail, "creditLevel")
		if sellerCredit == "" {
			sellerCredit = getStr(args, "seller_credit")
		}
		if sellerCredit == "" {
			sellerCredit = getStr(args, "creditLevel")
		}
		if sellerCredit == "" {
			sellerCredit = getStr(ex, "creditLevel")
		}

		items = append(items, map[string]interface{}{
			"id":            itemID,
			"title":         title,
			"price":         price,
			"location":      location,
			"seller_name":   sellerName,
			"seller_id":     getStr(args, "seller_id"),
			"seller_credit": sellerCredit,
			"image":         getStr(detail, "picUrl"),
		})
	}

	return items
}

// ParseItemDetail parses item detail API response into a flat map.
func ParseItemDetail(rawData map[string]interface{}) map[string]interface{} {
	itemInfo := getNestedMap(rawData, "itemDO")
	sellerInfo := getNestedMap(rawData, "sellerInfoDO")

	var images []string
	if imgList, ok := itemInfo["imageList"].([]interface{}); ok {
		for _, img := range imgList {
			switch v := img.(type) {
			case string:
				images = append(images, v)
			case map[string]interface{}:
				images = append(images, getStr(v, "url"))
			}
		}
	}

	return map[string]interface{}{
		"id":             getStr(itemInfo, "itemId"),
		"title":          getStr(itemInfo, "title"),
		"price":          getStr(itemInfo, "price"),
		"original_price": getStr(itemInfo, "originalPrice"),
		"description":    getStr(itemInfo, "desc"),
		"location":       getStr(itemInfo, "area"),
		"category":       getStr(itemInfo, "categoryName"),
		"condition":      getStr(itemInfo, "stuffStatus"),
		"view_count":     getNumeric(itemInfo, "viewCount"),
		"want_count":     getNumeric(itemInfo, "wantCount"),
		"images":         images,
		"seller_name":    getStr(sellerInfo, "nickName"),
		"seller_id":      getStr(sellerInfo, "userId"),
		"seller_credit":  getStr(sellerInfo, "creditLevel"),
		"created_at":     getStr(itemInfo, "publishTime"),
	}
}

func getNestedMap(m map[string]interface{}, key string) map[string]interface{} {
	if m == nil {
		return nil
	}
	v, ok := m[key].(map[string]interface{})
	if !ok {
		return nil
	}
	return v
}

func getStr(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v := m[key]
	if v == nil {
		return ""
	}
	// Avoid scientific notation for large numeric IDs (e.g. seller_id)
	if f, ok := v.(float64); ok {
		if f == float64(int64(f)) {
			return fmt.Sprintf("%.0f", f)
		}
		return fmt.Sprintf("%g", f)
	}
	return fmt.Sprintf("%v", v)
}

func getNumeric(m map[string]interface{}, key string) interface{} {
	if m == nil {
		return 0
	}
	v := m[key]
	if v == nil {
		return 0
	}
	return v
}
