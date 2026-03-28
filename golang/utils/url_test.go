package utils

import "testing"

func TestItemURLBasic(t *testing.T) {
	if ItemURL("12345") != "https://www.goofish.com/item?id=12345" {
		t.Error("unexpected URL")
	}
}

func TestItemURLLongID(t *testing.T) {
	longID := "10236256872xxxx"
	expected := "https://www.goofish.com/item?id=" + longID
	if ItemURL(longID) != expected {
		t.Error("unexpected URL for long ID")
	}
}

func TestItemURLEmpty(t *testing.T) {
	if ItemURL("") != "https://www.goofish.com/item?id=" {
		t.Error("unexpected URL for empty ID")
	}
}

func TestShareURLBasic(t *testing.T) {
	if ShareURL("12345") != "https://h5.m.goofish.com/item?id=12345" {
		t.Error("unexpected share URL")
	}
}

func TestShareURLLongID(t *testing.T) {
	if ShareURL("881817338832") != "https://h5.m.goofish.com/item?id=881817338832" {
		t.Error("unexpected share URL for long ID")
	}
}

func TestShareURLEmpty(t *testing.T) {
	if ShareURL("") != "https://h5.m.goofish.com/item?id=" {
		t.Error("unexpected share URL for empty ID")
	}
}

func TestShareURLClickable(t *testing.T) {
	url := ShareURL("12345")
	for _, c := range url {
		if c == '{' || c == '}' {
			t.Error("URL contains raw JSON chars")
		}
	}
}
