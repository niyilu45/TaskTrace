// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktracedata

import (
	"errors"

	"golang.org/x/sys/windows"
)

func temporaryRenameError(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
