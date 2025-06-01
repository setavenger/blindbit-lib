package utils

import (
	"os/user"
	"strings"
)

func ResolvePath(path string) string {
	usr, _ := user.Current()
	dir := usr.HomeDir

	return strings.Replace(path, "~", dir, 1)
}
