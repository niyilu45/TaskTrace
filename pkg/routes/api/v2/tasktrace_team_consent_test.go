// SPDX-License-Identifier: AGPL-3.0-or-later

package apiv2

import (
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"
)

func TestTaskTraceTeamConsentHeaders(t *testing.T) {
	api, err := NewCanonicalAPI()
	require.NoError(t, err)
	paths := api.OpenAPI().Paths
	for _, operation := range []*huma.Operation{
		paths["/tasks/{task}/team/share"].Post,
		paths["/tasktrace/team/permissions"].Put,
		paths["/tasktrace/team/members/access"].Post,
	} {
		t.Run(operation.OperationID, func(t *testing.T) {
			headers := map[string]*huma.Param{}
			for _, parameter := range operation.Parameters {
				if parameter.In == "header" {
					headers[parameter.Name] = parameter
				}
			}
			for _, name := range []string{"X-TaskTrace-Elevate", "X-TaskTrace-Create-Share"} {
				require.Contains(t, headers, name)
				require.False(t, headers[name].Required)
				require.Equal(t, "boolean", headers[name].Schema.Type)
			}
		})
	}
}
