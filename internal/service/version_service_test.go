package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/buildinfo"
)

func TestCompareVersionStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		currentVersion  string
		latestVersion   string
		wantStatus      string
		wantIsLatest    bool
		wantHasUpdate   bool
		wantMessagePart string
	}{
		{
			name:            "exact latest release",
			currentVersion:  "v1.2.3",
			latestVersion:   "v1.2.3",
			wantStatus:      "latest",
			wantIsLatest:    true,
			wantHasUpdate:   false,
			wantMessagePart: "最新正式版本",
		},
		{
			name:            "update available",
			currentVersion:  "v1.2.2",
			latestVersion:   "v1.2.3",
			wantStatus:      "update_available",
			wantIsLatest:    false,
			wantHasUpdate:   true,
			wantMessagePart: "发现新版本",
		},
		{
			name:            "git describe ahead of latest tag",
			currentVersion:  "v1.2.3-4-gabcdef0",
			latestVersion:   "v1.2.3",
			wantStatus:      "ahead",
			wantIsLatest:    false,
			wantHasUpdate:   false,
			wantMessagePart: "基于最新 tag 之后的提交",
		},
		{
			name:            "dirty build on latest tag",
			currentVersion:  "v1.2.3-dirty",
			latestVersion:   "v1.2.3",
			wantStatus:      "ahead",
			wantIsLatest:    false,
			wantHasUpdate:   false,
			wantMessagePart: "包含未提交修改",
		},
		{
			name:            "uncomparable dev build",
			currentVersion:  "dev",
			latestVersion:   "v1.2.3",
			wantStatus:      "unknown",
			wantIsLatest:    false,
			wantHasUpdate:   false,
			wantMessagePart: "不是可比较的正式版本号",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := compareVersionStatus(tt.currentVersion, tt.latestVersion)
			if got.status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", got.status, tt.wantStatus)
			}
			if got.isLatest != tt.wantIsLatest {
				t.Fatalf("isLatest = %v, want %v", got.isLatest, tt.wantIsLatest)
			}
			if got.hasUpdate != tt.wantHasUpdate {
				t.Fatalf("hasUpdate = %v, want %v", got.hasUpdate, tt.wantHasUpdate)
			}
			if !strings.Contains(got.message, tt.wantMessagePart) {
				t.Fatalf("message = %q, want substring %q", got.message, tt.wantMessagePart)
			}
		})
	}
}

func TestVersionServiceGetStatusUsesLatestRelease(t *testing.T) {
	previousVersion := buildinfo.Version
	previousBuildTime := buildinfo.BuildTime
	buildinfo.Version = "v1.2.2"
	buildinfo.BuildTime = "2026-03-18T12:00:00Z"
	t.Cleanup(func() {
		buildinfo.Version = previousVersion
		buildinfo.BuildTime = previousBuildTime
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if got := r.Header.Get("User-Agent"); got == "" {
			t.Fatal("expected User-Agent header")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"tag_name":"v1.2.3","html_url":"https://github.com/xuzhougeng/citebox/releases/tag/v1.2.3","published_at":"2026-03-18T08:00:00Z"}`)
	}))
	defer server.Close()

	now := time.Date(2026, 3, 18, 12, 30, 0, 0, time.UTC)
	svc := NewVersionService(
		WithVersionHTTPClient(server.Client()),
		WithVersionReleaseEndpoint(server.URL, "https://github.com/xuzhougeng/citebox/releases/latest"),
		WithVersionNow(func() time.Time { return now }),
	)

	got := svc.GetStatus(context.Background(), false)
	if got.CurrentVersion != "v1.2.2" {
		t.Fatalf("current version = %q, want %q", got.CurrentVersion, "v1.2.2")
	}
	if got.BuildTime != "2026-03-18T12:00:00Z" {
		t.Fatalf("build time = %q, want %q", got.BuildTime, "2026-03-18T12:00:00Z")
	}
	if got.LatestVersion != "v1.2.3" {
		t.Fatalf("latest version = %q, want %q", got.LatestVersion, "v1.2.3")
	}
	if got.Status != "update_available" {
		t.Fatalf("status = %q, want %q", got.Status, "update_available")
	}
	if !got.HasUpdate {
		t.Fatal("expected HasUpdate = true")
	}
	if got.CheckedAt != now.Format(time.RFC3339) {
		t.Fatalf("checked_at = %q, want %q", got.CheckedAt, now.Format(time.RFC3339))
	}
}

func TestVersionServiceGetStatusFallsBackToCacheWhenRefreshFails(t *testing.T) {
	previousVersion := buildinfo.Version
	previousBuildTime := buildinfo.BuildTime
	buildinfo.Version = "v1.2.3"
	buildinfo.BuildTime = "2026-03-18T12:00:00Z"
	t.Cleanup(func() {
		buildinfo.Version = previousVersion
		buildinfo.BuildTime = previousBuildTime
	})

	shouldFail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if shouldFail {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"rate limited"}`)
			return
		}
		fmt.Fprint(w, `{"tag_name":"v1.2.3","html_url":"https://github.com/xuzhougeng/citebox/releases/tag/v1.2.3","published_at":"2026-03-18T08:00:00Z"}`)
	}))
	defer server.Close()

	now := time.Date(2026, 3, 18, 12, 30, 0, 0, time.UTC)
	svc := NewVersionService(
		WithVersionHTTPClient(server.Client()),
		WithVersionReleaseEndpoint(server.URL, "https://github.com/xuzhougeng/citebox/releases/latest"),
		WithVersionNow(func() time.Time { return now }),
	)

	initial := svc.GetStatus(context.Background(), false)
	if initial.Status != "latest" {
		t.Fatalf("initial status = %q, want %q", initial.Status, "latest")
	}

	shouldFail = true
	refreshed := svc.GetStatus(context.Background(), true)
	if refreshed.Status != "latest" {
		t.Fatalf("refreshed status = %q, want cached %q", refreshed.Status, "latest")
	}
	if !strings.Contains(refreshed.Message, "刷新失败") {
		t.Fatalf("refreshed message = %q, want refresh failure notice", refreshed.Message)
	}
	if refreshed.LatestVersion != "v1.2.3" {
		t.Fatalf("latest version = %q, want cached %q", refreshed.LatestVersion, "v1.2.3")
	}
}
