package services

import (
	"context"
	"testing"
	"time"

	apimodels "github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/errs"
)

func TestBuildUserExportAssemblesProfileAndNotes(t *testing.T) {
	profile := &apimodels.UserObj{UserId: "u1", Nickname: "reader"}
	settings := &apimodels.UserSettings{Language: "zh"}
	now := time.Unix(1_700_000_000, 0)

	out := BuildUserExport(ExportInput{Profile: profile, Settings: settings}, now)

	if out.UserId != "u1" || out.Profile != profile {
		t.Fatalf("profile not carried through: %+v", out)
	}
	if out.Settings != settings {
		t.Fatal("settings not carried through")
	}
	if out.ExportedAt != now.UnixMilli() {
		t.Fatalf("exported_at = %d", out.ExportedAt)
	}
	if len(out.Notes) == 0 {
		t.Fatal("export must explain what is missing")
	}
	if out.Friends == nil || out.Groups == nil || out.BlockedUsers == nil || out.Reports == nil {
		t.Fatal("empty sections must be empty slices, not null")
	}
	if out.Truncated != nil {
		t.Fatalf("nothing was truncated: %+v", out.Truncated)
	}
}

func TestBuildUserExportMarksTruncation(t *testing.T) {
	profile := &apimodels.UserObj{UserId: "u1"}
	friends := make([]*apimodels.UserObj, exportMaxFriends+5)
	for i := range friends {
		friends[i] = &apimodels.UserObj{UserId: "f"}
	}

	out := BuildUserExport(ExportInput{Profile: profile, Friends: friends}, time.Now())

	if len(out.Friends) != exportMaxFriends {
		t.Fatalf("friends capped at %d, got %d", exportMaxFriends, len(out.Friends))
	}
	if out.Truncated == nil || !out.Truncated.Friends {
		t.Fatal("truncation must be reported to the user")
	}
	if len(friends) != exportMaxFriends+5 {
		t.Fatal("input slice must not be modified")
	}
}

func TestBuildUserExportCapsReportsAndBlocks(t *testing.T) {
	reports := make([]*apimodels.ReportItem, exportMaxReports+1)
	for i := range reports {
		reports[i] = &apimodels.ReportItem{Id: int64(i)}
	}
	blocks := make([]*apimodels.UserObj, exportMaxBlocks+1)
	for i := range blocks {
		blocks[i] = &apimodels.UserObj{UserId: "b"}
	}

	out := BuildUserExport(ExportInput{Reports: reports, Blocks: blocks}, time.Now())

	if len(out.Reports) != exportMaxReports || len(out.BlockedUsers) != exportMaxBlocks {
		t.Fatalf("caps not applied: reports=%d blocks=%d", len(out.Reports), len(out.BlockedUsers))
	}
	if out.Truncated == nil || !out.Truncated.Reports || !out.Truncated.BlockedUsers {
		t.Fatal("both truncations must be reported")
	}
	if out.Truncated.Groups || out.Truncated.Friends {
		t.Fatal("untouched sections must not be marked")
	}
	// 每个调用方拿到独立的 notes 副本，改一个不会污染全局。
	out.Notes[0] = "mutated"
	if exportNotes[0] == "mutated" {
		t.Fatal("notes must be copied per export")
	}
}

func TestExportUserDataRequiresLogin(t *testing.T) {
	code, export := ExportUserData(context.Background())
	if code != errs.IMErrorCode_APP_NOT_LOGIN {
		t.Fatalf("got %d, want %d", code, errs.IMErrorCode_APP_NOT_LOGIN)
	}
	if export != nil {
		t.Fatal("export without requester must not return data")
	}
}
