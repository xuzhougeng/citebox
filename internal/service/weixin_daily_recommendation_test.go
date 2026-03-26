package service

import (
	"context"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/model"
)

func TestValidateWeixinDailyRecommendationSettingsDefaultsTime(t *testing.T) {
	settings, err := validateWeixinDailyRecommendationSettings(model.WeixinDailyRecommendationSettings{
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("validateWeixinDailyRecommendationSettings() error = %v", err)
	}
	if settings.SendTime != model.DefaultWeixinDailyRecommendationSendTime {
		t.Fatalf("validateWeixinDailyRecommendationSettings() send_time = %q, want %q", settings.SendTime, model.DefaultWeixinDailyRecommendationSendTime)
	}
}

func TestValidateWeixinDailyRecommendationSettingsRejectsInvalidTime(t *testing.T) {
	_, err := validateWeixinDailyRecommendationSettings(model.WeixinDailyRecommendationSettings{
		Enabled:  true,
		SendTime: "25:99",
	})
	if !apperr.IsCode(err, apperr.CodeInvalidArgument) {
		t.Fatalf("validateWeixinDailyRecommendationSettings() code = %q, want %q", apperr.CodeOf(err), apperr.CodeInvalidArgument)
	}
}

func TestShouldSendWeixinDailyRecommendation(t *testing.T) {
	now := time.Date(2026, 3, 26, 9, 30, 0, 0, time.Local)
	if !shouldSendWeixinDailyRecommendation(now, "09:00", "") {
		t.Fatal("shouldSendWeixinDailyRecommendation() = false, want true after configured time")
	}
	if shouldSendWeixinDailyRecommendation(now, "10:00", "") {
		t.Fatal("shouldSendWeixinDailyRecommendation() = true, want false before configured time")
	}
	if shouldSendWeixinDailyRecommendation(now, "09:00", now.Format(weixinDailyRecommendationDateLayout)) {
		t.Fatal("shouldSendWeixinDailyRecommendation() = true, want false after today's send state is recorded")
	}
}

func TestMaybeSendWeixinDailyRecommendationSkipsWhenBridgeDisabled(t *testing.T) {
	svc, _, _ := newTestService(t)

	result, err := svc.MaybeSendWeixinDailyRecommendation(context.Background(), time.Date(2026, 3, 26, 9, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("MaybeSendWeixinDailyRecommendation() error = %v", err)
	}
	if result != nil {
		t.Fatalf("MaybeSendWeixinDailyRecommendation() = %+v, want nil when bridge is disabled", result)
	}
}

func TestTestWeixinDailyRecommendationRequiresBinding(t *testing.T) {
	svc, _, _ := newTestService(t)

	_, err := svc.TestWeixinDailyRecommendation(context.Background(), model.WeixinDailyRecommendationSettings{
		SendTime: "09:00",
	})
	if !apperr.IsCode(err, apperr.CodeFailedPrecondition) {
		t.Fatalf("TestWeixinDailyRecommendation() code = %q, want %q", apperr.CodeOf(err), apperr.CodeFailedPrecondition)
	}
}
