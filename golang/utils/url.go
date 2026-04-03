package utils

import "fmt"

const (
	goofishItemURL = "https://www.goofish.com/item?id=%s"
	goofishH5URL   = "https://h5.m.goofish.com/item?id=%s"
)

// ItemURL builds a Goofish item URL from an item ID.
func ItemURL(itemID string) string {
	return fmt.Sprintf(goofishItemURL, itemID)
}

// ShareURL builds a mobile-friendly Goofish share URL.
func ShareURL(itemID string) string {
	return fmt.Sprintf(goofishH5URL, itemID)
}
