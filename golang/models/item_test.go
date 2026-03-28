package models

import "testing"

func mockAPIResponse() map[string]interface{} {
	return map[string]interface{}{
		"resultList": []interface{}{
			map[string]interface{}{
				"data": map[string]interface{}{
					"item": map[string]interface{}{
						"main": map[string]interface{}{
							"exContent": map[string]interface{}{
								"area": "杭州",
								"detailParams": map[string]interface{}{
									"itemId":    "123456",
									"title":     "iPhone 15 Pro 256G",
									"soldPrice": "5999",
									"userNick":  "测试卖家",
								},
							},
							"clickParam": map[string]interface{}{
								"args": map[string]interface{}{
									"id":        "123456",
									"price":     "5999",
									"p_city":    "杭州市",
									"seller_id": "user001",
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestParseSearchItems(t *testing.T) {
	items := ParseSearchItems(mockAPIResponse())
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	item := items[0]
	if item["id"] != "123456" {
		t.Errorf("unexpected id: %v", item["id"])
	}
	if item["title"] != "iPhone 15 Pro 256G" {
		t.Errorf("unexpected title: %v", item["title"])
	}
	if item["price"] != "5999" {
		t.Errorf("unexpected price: %v", item["price"])
	}
	if item["location"] != "杭州" {
		t.Errorf("unexpected location: %v", item["location"])
	}
	if item["seller_name"] != "测试卖家" {
		t.Errorf("unexpected seller_name: %v", item["seller_name"])
	}
}

func TestParseSearchItemsEmpty(t *testing.T) {
	items := ParseSearchItems(map[string]interface{}{})
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseSearchItemsNoItemData(t *testing.T) {
	data := map[string]interface{}{
		"resultList": []interface{}{
			map[string]interface{}{"data": map[string]interface{}{"item": map[string]interface{}{}}},
		},
	}
	items := ParseSearchItems(data)
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseItemDetail(t *testing.T) {
	raw := map[string]interface{}{
		"itemDO": map[string]interface{}{
			"itemId": "789", "title": "MacBook Pro", "price": "8999.00",
			"desc": "9成新", "area": "上海",
			"imageList": []interface{}{"http://img1.jpg", "http://img2.jpg"},
			"viewCount": 100, "wantCount": 5,
		},
		"sellerInfoDO": map[string]interface{}{
			"nickName": "卖家A", "userId": "seller001",
		},
	}
	detail := ParseItemDetail(raw)
	if detail["id"] != "789" {
		t.Errorf("unexpected id: %v", detail["id"])
	}
	if detail["title"] != "MacBook Pro" {
		t.Errorf("unexpected title: %v", detail["title"])
	}
	if detail["description"] != "9成新" {
		t.Errorf("unexpected description: %v", detail["description"])
	}
	images := detail["images"].([]string)
	if len(images) != 2 {
		t.Errorf("expected 2 images, got %d", len(images))
	}
	if detail["seller_name"] != "卖家A" {
		t.Errorf("unexpected seller: %v", detail["seller_name"])
	}
}
