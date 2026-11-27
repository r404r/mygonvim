package loginenv

import (
	"os"

	"golang.org/x/sys/unix"
)

func StdoutTTY() bool {
	_, err := unix.IoctlGetTermios(int(os.Stdout.Fd()), unix.TIOCGETA)
	return err == nil
}
