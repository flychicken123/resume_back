package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"resumeai/models"
)

type FeedbackService struct {
	model        *models.FeedbackModel
	email        *EmailService
	logger       *log.Logger
	pollInterval time.Duration
}

func NewFeedbackService(model *models.FeedbackModel, email *EmailService, logger *log.Logger) *FeedbackService {
	return &FeedbackService{
		model:        model,
		email:        email,
		logger:       logger,
		pollInterval: time.Hour,
	}
}

func (s *FeedbackService) StartScheduler(ctx context.Context) {
	if s == nil || s.model == nil || s.email == nil || !s.email.Enabled() {
		return
	}

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	s.runOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

func (s *FeedbackService) runOnce(ctx context.Context) {
	items, err := s.model.FetchDueFollowUps(ctx, 25)
	if err != nil {
		s.logger.Printf("feedback scheduler: fetch error: %v", err)
		return
	}
	if len(items) == 0 {
		return
	}

	for _, item := range items {
		if err := s.dispatchFollowUp(ctx, item); err != nil {
			s.logger.Printf("feedback scheduler: send error id=%d err=%v", item.ID, err)
			_ = s.model.MarkFollowUpFailed(ctx, item.ID, err.Error())
		} else {
			_ = s.model.MarkFollowUpSent(ctx, item.ID)
		}
	}
}

func (s *FeedbackService) dispatchFollowUp(ctx context.Context, item models.FeedbackFollowUp) error {
	if !s.email.Enabled() {
		return fmt.Errorf("email service not configured")
	}

	subject, body := buildFollowUpMessage(item)
	if subject == "" || body == "" {
		return fmt.Errorf("no template for trigger %s", item.TriggerKey)
	}
	return s.email.SendEmail(item.UserEmail, subject, body)
}

func buildFollowUpMessage(item models.FeedbackFollowUp) (string, string) {
	metadata := item.Metadata
	switch item.TriggerKey {
	case "resume_download":
		return "How did your resume download go?", fmt.Sprintf(`Hi there,\n\nWe noticed you downloaded a resume from HiHired recently. We'd love to hear if it matched what you needed or if something felt off.\n\nShare your thoughts so we can keep improving: https://hihired.org/contact\n\nThanks for helping us build a better experience!\n- The HiHired Team`)
	case "job_match":
		jobTitle := "your job search"
		if metadata != nil {
			if title, ok := metadata["job_title"].(string); ok && title != "" {
				jobTitle = title
			}
		}
		return "Still curious about those job matches?", fmt.Sprintf(`Hello,\n\nYesterday we suggested roles for %s. Did any stand out? If not, tell us what we missed and we'll fine-tune the recommendations.\n\nReply to this email or share feedback directly in the app.\n\nWe're in your corner,\n- The HiHired Team`, jobTitle)
	default:
		payload, _ := json.Marshal(metadata)
		return "We'd love your feedback", fmt.Sprintf(`Hi,\n\nWe'd love to hear a quick update on your experience with HiHired.\n\nContext: %s\n\nLet us know how we can improve!\n\n— HiHired`, string(payload))
	}
}
