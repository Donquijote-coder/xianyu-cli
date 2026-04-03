"""Tests for search item parsing."""

from xianyu_cli.models.item import parse_item_detail, parse_search_items


def test_parse_search_items(mock_api_response):
    items = parse_search_items(mock_api_response["data"])
    assert len(items) == 1
    assert items[0]["id"] == "123456"
    assert items[0]["title"] == "iPhone 15 Pro 256G"
    assert items[0]["price"] == "5999"  # soldPrice is in yuan
    assert items[0]["location"] == "杭州"
    assert items[0]["seller_name"] == "测试卖家"


def test_parse_search_items_empty():
    items = parse_search_items({})
    assert items == []


def test_parse_search_items_no_item_data():
    data = {"resultList": [{"data": {"item": {}}}]}
    items = parse_search_items(data)
    assert items == []


def test_parse_item_detail():
    raw = {
        "itemDO": {
            "itemId": "789",
            "title": "MacBook Pro",
            "price": "8999.00",
            "desc": "9成新",
            "area": "上海",
            "imageList": ["http://img1.jpg", "http://img2.jpg"],
            "viewCount": 100,
            "wantCount": 5,
        },
        "sellerInfoDO": {
            "nickName": "卖家A",
            "userId": "seller001",
        },
    }

    detail = parse_item_detail(raw)
    assert detail["id"] == "789"
    assert detail["title"] == "MacBook Pro"
    assert detail["price"] == "8999.00"
    assert detail["description"] == "9成新"
    assert len(detail["images"]) == 2
    assert detail["seller_name"] == "卖家A"
    assert detail["view_count"] == 100
