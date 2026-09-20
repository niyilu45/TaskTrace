// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"errors"
	"reflect"
	"testing"
)

func TestTaskTraceTeamMemberLinkRoundTrip(t *testing.T) {
	link := taskTraceTeamEncodeMembersLink([]string{`OFFICE\Alice`, "alice@example.test", "Bob"})
	members, err := taskTraceTeamDecodeMembersLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(members, []string{"Alice", "Bob"}) {
		t.Fatalf("unexpected members: %#v", members)
	}
}

func TestTaskTraceTeamImportMemberNamesKeepsPartialSuccess(t *testing.T) {
	added, skipped, failed := taskTraceTeamImportMemberNames(
		[]string{`OFFICE\Current`, `OFFICE\Alice`, "missing", "alice"},
		"current",
		func(member string) (string, error) {
			if member == "missing" {
				return "", errors.New("not found")
			}
			return member, nil
		},
	)
	if !reflect.DeepEqual(added, []string{`OFFICE\Alice`}) {
		t.Fatalf("unexpected added members: %#v", added)
	}
	if !reflect.DeepEqual(skipped, []string{`OFFICE\Current`}) {
		t.Fatalf("unexpected skipped members: %#v", skipped)
	}
	if len(failed) != 1 || failed[0].Username != "missing" {
		t.Fatalf("unexpected failures: %#v", failed)
	}
}

func TestTaskTraceTeamUnassignedMembers(t *testing.T) {
	state := taskTraceTeamState{Bindings: []TaskTraceTeamBinding{{Owner: "owner", Members: []string{"Alice"}}}}
	got := taskTraceTeamUnassignedMembers(state, []string{`OFFICE\Alice`, `OFFICE\Bob`, `OFFICE\Current`}, "current")
	want := []string{`OFFICE\Bob`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected unassigned members: got %#v want %#v", got, want)
	}
}
