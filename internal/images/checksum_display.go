package images

const hashDisplayHexLength = 16

func hashDisplayPrefix(value string) string {
	if len(value) <= hashDisplayHexLength {
		return value
	}
	return value[:hashDisplayHexLength]
}
