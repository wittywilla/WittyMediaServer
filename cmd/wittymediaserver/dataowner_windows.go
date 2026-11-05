package main

// dataOwnerOK has nothing to check on Windows, where new files take the folder's permissions.
func dataOwnerOK(string) error { return nil }

func ownerOf(string) string { return "" }
