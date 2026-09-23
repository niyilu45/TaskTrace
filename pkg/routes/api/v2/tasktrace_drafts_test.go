// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package apiv2

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskTraceDraftReplaceAndRecoverInterruptedWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "draft.json")
	require.NoError(t, taskTraceDraftReplace(path, `{"progress":"first"}`))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.JSONEq(t, `{"progress":"first"}`, string(content))

	require.NoError(t, os.Rename(path, path+".bak"))
	require.NoFileExists(t, path)
	require.NoError(t, taskTraceDraftRecover(path))
	content, err = os.ReadFile(path)
	require.NoError(t, err)
	require.JSONEq(t, `{"progress":"first"}`, string(content))

	require.NoError(t, taskTraceDraftReplace(path, `{"progress":"second"}`))
	content, err = os.ReadFile(path)
	require.NoError(t, err)
	require.JSONEq(t, `{"progress":"second"}`, string(content))
	require.NoFileExists(t, path+".bak")
}
