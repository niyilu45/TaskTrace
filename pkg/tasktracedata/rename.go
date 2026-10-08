// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktracedata

import (
	"os"
	"time"
)

// Antivirus/indexer handles can briefly block Windows renames after a copy.
// Retry only those errors. Never remove the old destination to force a replace.
func renameDataFile(source, destination string) error {
	return retryDataRename(func() error { return os.Rename(source, destination) })
}

func retryDataRename(rename func() error) error {
	delays := [...]time.Duration{50, 100, 200, 400, 800, 1000}
	for attempt := 0; ; attempt++ {
		err := rename()
		if err == nil || !temporaryRenameError(err) || attempt == len(delays) {
			return err
		}
		time.Sleep(delays[attempt] * time.Millisecond)
	}
}
