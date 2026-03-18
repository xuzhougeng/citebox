package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"

	"github.com/xuzhougeng/citebox/internal/buildinfo"
	"github.com/xuzhougeng/citebox/internal/model"
)

const versionStatusCacheTTL = 10 * time.Minute

var gitDescribeVersionPattern = regexp.MustCompile(`^(v?\d+\.\d+\.\d+)(?:-([0-9]+)-g[0-9a-f]+)?(?:-dirty)?$`)

type VersionService struct {
	httpClient       *http.Client
	now              func() time.Time
	latestReleaseAPI string
	releasesPageURL  string
	cacheTTL         time.Duration
	mu               sync.Mutex
	cachedStatus     model.VersionStatus
	cachedAt         time.Time
	hasCachedStatus  bool
}

type VersionServiceOption func(*VersionService)

type githubLatestReleaseResponse struct {
	TagName     string    `json:"tag_name"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Message     string    `json:"message"`
}

type parsedComparableVersion struct {
	base         string
	commitsAhead int
	dirty        bool
}

type versionComparison struct {
	status    string
	isLatest  bool
	hasUpdate bool
	message   string
}

func NewVersionService(opts ...VersionServiceOption) *VersionService {
	svc := &VersionService{
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		now:              time.Now,
		latestReleaseAPI: buildinfo.LatestReleaseAPI,
		releasesPageURL:  buildinfo.ReleasesPageURL,
		cacheTTL:         versionStatusCacheTTL,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
}

func WithVersionHTTPClient(client *http.Client) VersionServiceOption {
	return func(s *VersionService) {
		if client != nil {
			s.httpClient = client
		}
	}
}

func WithVersionNow(now func() time.Time) VersionServiceOption {
	return func(s *VersionService) {
		if now != nil {
			s.now = now
		}
	}
}

func WithVersionReleaseEndpoint(apiURL, pageURL string) VersionServiceOption {
	return func(s *VersionService) {
		if strings.TrimSpace(apiURL) != "" {
			s.latestReleaseAPI = strings.TrimSpace(apiURL)
		}
		if strings.TrimSpace(pageURL) != "" {
			s.releasesPageURL = strings.TrimSpace(pageURL)
		}
	}
}

func (s *VersionService) GetStatus(ctx context.Context, forceRefresh bool) model.VersionStatus {
	if !forceRefresh {
		if cached, ok := s.loadCachedStatus(); ok {
			return cached
		}
	}

	release, err := s.fetchLatestRelease(ctx)
	if err != nil {
		if cached, ok := s.loadAnyCachedStatus(); ok {
			cached.Message = fmt.Sprintf("%s；本次刷新失败：%s", cached.Message, err.Error())
			return cached
		}

		status := s.baseStatus()
		status.CheckedAt = s.now().UTC().Format(time.RFC3339)
		status.Status = "unknown"
		status.Message = "暂时无法获取最新版本信息：" + err.Error()
		return status
	}

	status := s.baseStatus()
	status.CheckedAt = s.now().UTC().Format(time.RFC3339)
	status.LatestVersion = strings.TrimSpace(release.TagName)
	status.LatestReleaseURL = firstNonEmptyString(strings.TrimSpace(release.HTMLURL), s.releasesPageURL)
	if !release.PublishedAt.IsZero() {
		status.PublishedAt = release.PublishedAt.UTC().Format(time.RFC3339)
	}

	comparison := compareVersionStatus(status.CurrentVersion, status.LatestVersion)
	status.Status = comparison.status
	status.IsLatest = comparison.isLatest
	status.HasUpdate = comparison.hasUpdate
	status.Message = comparison.message

	s.storeCachedStatus(status)
	return status
}

func (s *VersionService) baseStatus() model.VersionStatus {
	return model.VersionStatus{
		CurrentVersion: buildinfo.CurrentVersion(),
		BuildTime:      buildinfo.CurrentBuildTime(),
		Status:         "unknown",
		Message:        "尚未检查最新版本",
	}
}

func (s *VersionService) fetchLatestRelease(ctx context.Context) (*githubLatestReleaseResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.latestReleaseAPI, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "citebox-version-check")

	response, err := s.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	var payload githubLatestReleaseResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析版本响应失败: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(payload.Message)
		if message == "" {
			message = response.Status
		}
		return nil, fmt.Errorf("GitHub Release 查询失败: %s", message)
	}

	if strings.TrimSpace(payload.TagName) == "" {
		return nil, fmt.Errorf("GitHub Release 未返回 tag_name")
	}

	return &payload, nil
}

func (s *VersionService) loadCachedStatus() (model.VersionStatus, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.hasCachedStatus {
		return model.VersionStatus{}, false
	}
	if s.cacheTTL <= 0 || s.now().Sub(s.cachedAt) > s.cacheTTL {
		return model.VersionStatus{}, false
	}
	return s.cachedStatus, true
}

func (s *VersionService) loadAnyCachedStatus() (model.VersionStatus, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.hasCachedStatus {
		return model.VersionStatus{}, false
	}
	return s.cachedStatus, true
}

func (s *VersionService) storeCachedStatus(status model.VersionStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cachedStatus = status
	s.cachedAt = s.now()
	s.hasCachedStatus = true
}

func compareVersionStatus(currentVersion, latestVersion string) versionComparison {
	currentRaw := strings.TrimSpace(currentVersion)
	latestRaw := strings.TrimSpace(latestVersion)
	if currentRaw == "" {
		return versionComparison{
			status:  "unknown",
			message: "当前构建未携带版本号，无法比较",
		}
	}
	if currentRaw == latestRaw {
		return versionComparison{
			status:   "latest",
			isLatest: true,
			message:  "当前已是最新正式版本",
		}
	}

	latestNormalized := normalizeSemver(latestRaw)
	if latestNormalized == "" {
		return versionComparison{
			status:  "unknown",
			message: "最新版本号格式无法识别，暂时不能比较",
		}
	}

	currentComparable, ok := parseComparableVersion(currentRaw)
	if !ok {
		return versionComparison{
			status:  "unknown",
			message: fmt.Sprintf("当前版本 %s 不是可比较的正式版本号", currentRaw),
		}
	}

	baseComparison := semver.Compare(currentComparable.base, latestNormalized)
	switch {
	case baseComparison < 0:
		return versionComparison{
			status:    "update_available",
			hasUpdate: true,
			message:   fmt.Sprintf("发现新版本 %s，可前往 Release 页面下载", latestRaw),
		}
	case baseComparison > 0:
		return versionComparison{
			status:  "ahead",
			message: "当前构建版本高于最新正式发布版本",
		}
	}

	if currentComparable.commitsAhead > 0 {
		return versionComparison{
			status:  "ahead",
			message: "当前构建基于最新 tag 之后的提交，已不落后于最新正式版本",
		}
	}
	if currentComparable.dirty {
		return versionComparison{
			status:  "ahead",
			message: "当前构建基于最新 tag，但包含未提交修改",
		}
	}

	return versionComparison{
		status:   "latest",
		isLatest: true,
		message:  "当前已是最新正式版本",
	}
}

func parseComparableVersion(raw string) (parsedComparableVersion, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return parsedComparableVersion{}, false
	}

	dirty := strings.HasSuffix(value, "-dirty")

	matches := gitDescribeVersionPattern.FindStringSubmatch(value)
	if len(matches) > 0 {
		base := normalizeSemver(matches[1])
		if base == "" {
			return parsedComparableVersion{}, false
		}

		commitsAhead := 0
		if matches[2] != "" {
			parsed, err := strconv.Atoi(matches[2])
			if err != nil {
				return parsedComparableVersion{}, false
			}
			commitsAhead = parsed
		}

		return parsedComparableVersion{
			base:         base,
			commitsAhead: commitsAhead,
			dirty:        dirty,
		}, true
	}

	trimmed := strings.TrimSuffix(value, "-dirty")
	if normalized := normalizeSemver(trimmed); normalized != "" {
		return parsedComparableVersion{
			base:  normalized,
			dirty: dirty,
		}, true
	}

	return parsedComparableVersion{}, false
}

func normalizeSemver(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "v") {
		value = "v" + value
	}
	if !semver.IsValid(value) {
		return ""
	}
	return semver.Canonical(value)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
