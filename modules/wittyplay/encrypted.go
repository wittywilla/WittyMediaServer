package wittyplay

// Refusal says why a file for this console can't be played, in words for a person,
// or "" when nothing is known against it. A file that can't be read is a different
// problem, and whoever opens it next says so. Nothing here decrypts anything or tries
// to: it only reads what a file's headers say in the open (see threeds.go).
func Refusal(path, consoleID string) string {
	if consoleID != "3ds" {
		return ""
	}
	info, err := Inspect3DS(path)
	if err != nil || !info.Encrypted {
		return ""
	}
	return ErrEncrypted3DS.Error()
}
