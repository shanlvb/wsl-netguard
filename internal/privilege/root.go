package privilege

import (
	"errors"
	"fmt"
)

var ErrRootRequired = errors.New("root privileges required")

func RequireRoot(euid int) error {
	if euid == 0 {
		return nil
	}
	return fmt.Errorf("%w: rerun this command with sudo", ErrRootRequired)
}
