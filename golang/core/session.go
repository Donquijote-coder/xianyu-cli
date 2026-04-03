package core

import (
	"fmt"
	"log"
	"strconv"

	"xianyu-cli/utils"
)

// RunAPICall executes an API call with automatic token refresh and cookie persistence.
func RunAPICall(cred *utils.Credential, api string, data map[string]interface{}, version ...string) (map[string]interface{}, error) {
	v := "1.0"
	if len(version) > 0 {
		v = version[0]
	}

	client := NewGoofishApiClient(cred.Cookies)
	defer client.Close()

	result, err := client.Call(api, data, v)
	if err != nil {
		return nil, err
	}

	// Persist cookies when the token was refreshed
	if client.TokenRefreshed {
		log.Printf("Token was refreshed, saving updated credential")
		if tk, ok := client.Cookies["_m_h5_tk"]; ok {
			cred.Cookies["_m_h5_tk"] = tk
		}
		if tkEnc, ok := client.Cookies["_m_h5_tk_enc"]; ok {
			cred.Cookies["_m_h5_tk_enc"] = tkEnc
		}
		utils.SaveCredential(cred)
	}

	return result, nil
}

// ParseCredit converts a raw creditLevel value to a sortable integer.
func ParseCredit(raw interface{}) int {
	switch v := raw.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		digits := ""
		for _, c := range v {
			if c >= '0' && c <= '9' {
				digits += string(c)
			}
		}
		if digits != "" {
			n, _ := strconv.Atoi(digits)
			return n
		}
	}
	return 0
}

// EnrichSellerCredit fetches seller credit for each item via the detail API.
func EnrichSellerCredit(cred *utils.Credential, items []map[string]interface{}, concurrency int) []map[string]interface{} {
	if concurrency <= 0 {
		concurrency = 5
	}

	sem := make(chan struct{}, concurrency)
	done := make(chan struct{}, len(items))

	for i := range items {
		go func(item map[string]interface{}) {
			sem <- struct{}{}
			defer func() { <-sem; done <- struct{}{} }()

			// Skip if already has credit
			if credit, ok := item["seller_credit"]; ok && credit != nil && credit != "" && credit != 0 {
				return
			}

			itemID, _ := item["id"].(string)
			if itemID == "" {
				item["seller_credit"] = 0
				return
			}

			detail, err := RunAPICall(cred, "mtop.taobao.idle.pc.detail", map[string]interface{}{"itemId": itemID})
			if err != nil {
				item["seller_credit"] = 0
				return
			}

			seller, _ := detail["sellerDO"].(map[string]interface{})
			if seller == nil {
				item["seller_credit"] = 0
				return
			}

			creditTag, _ := seller["idleFishCreditTag"].(map[string]interface{})
			trackParams, _ := creditTag["trackParams"].(map[string]interface{})
			sellerLevel := fmt.Sprintf("%v", trackParams["sellerLevel"])
			item["seller_credit"] = ParseCredit(sellerLevel)

			if goodRate, ok := seller["newGoodRatioRate"]; ok {
				item["seller_good_rate"] = fmt.Sprintf("%v", goodRate)
			}
			if soldCount, ok := seller["hasSoldNumInteger"]; ok {
				item["seller_sold_count"] = soldCount
			}

			zhima, _ := seller["zhimaLevelInfo"].(map[string]interface{})
			if levelName, ok := zhima["levelName"]; ok {
				item["zhima_credit"] = fmt.Sprintf("%v", levelName)
			}
		}(items[i])
	}

	for range items {
		<-done
	}

	return items
}
