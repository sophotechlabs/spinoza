package imagepin

import "strings"

const separator = "@sha256:"

func Valid(image string) bool {
	at := strings.LastIndex(image, separator)
	if at <= 0 {
		return false
	}
	digest := image[at+len(separator):]
	if len(digest) != 64 {
		return false
	}
	for _, letter := range digest {
		decimal := letter >= '0' && letter <= '9'
		lowerHex := letter >= 'a' && letter <= 'f'
		if !decimal && !lowerHex {
			return false
		}
	}
	return true
}
