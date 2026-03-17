"""Tests for URL utility."""

from xianyu_cli.utils.url import item_url


def test_item_url_basic():
    assert item_url("12345") == "https://www.goofish.com/item?id=12345"


def test_item_url_long_id():
    long_id = "10236256872xxxx"
    assert item_url(long_id) == f"https://www.goofish.com/item?id={long_id}"


def test_item_url_empty():
    assert item_url("") == "https://www.goofish.com/item?id="
