package services

import (
	"context"
	"errors"
	"hash/fnv"
	"strings"
	"time"

	"resumeai/models"
)

type ExperimentService struct {
	model *models.ExperimentModel
}

type AssignmentResult struct {
	Experiment models.Experiment           `json:"experiment"`
	Variant    models.ExperimentVariant    `json:"variant"`
	Assignment models.ExperimentAssignment `json:"assignment"`
}

type TrackEventRequest struct {
	ExperimentKey string                 `json:"experiment_key"`
	EventName     string                 `json:"event_name"`
	UserID        string                 `json:"user_id"`
	VariantKey    string                 `json:"variant_key,omitempty"`
	RequestPath   string                 `json:"request_path,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

func NewExperimentService(model *models.ExperimentModel) *ExperimentService {
	return &ExperimentService{model: model}
}

func (s *ExperimentService) UpsertExperiment(ctx context.Context, exp models.Experiment, variants []models.ExperimentVariant) (*models.ExperimentWithVariants, error) {
	return s.model.UpsertExperiment(ctx, exp, variants)
}

func (s *ExperimentService) ListExperiments(ctx context.Context) ([]models.ExperimentWithVariants, error) {
	return s.model.ListExperiments(ctx)
}

func (s *ExperimentService) GetExperiment(ctx context.Context, key string) (*models.ExperimentWithVariants, error) {
	return s.model.GetByKey(ctx, key)
}

func (s *ExperimentService) GetExperimentWithMetrics(ctx context.Context, key string) (*models.ExperimentWithVariants, error) {
	experiment, err := s.model.GetByKey(ctx, key)
	if err != nil {
		return nil, err
	}

	metrics, err := s.model.GetMetrics(ctx, experiment.Experiment.ID)
	if err != nil {
		return nil, err
	}
	experiment.Metrics = metrics

	return experiment, nil
}

func (s *ExperimentService) DeleteExperiment(ctx context.Context, key string) error {
	return s.model.DeleteByKey(ctx, key)
}

func (s *ExperimentService) AssignVariant(ctx context.Context, experimentKey, userIdentifier, requestPath string, forceReassign bool) (*AssignmentResult, error) {
	experiment, err := s.model.GetByKey(ctx, experimentKey)
	if err != nil {
		return nil, err
	}

	if forceReassign {
		_ = s.model.DeleteAssignment(ctx, experiment.Experiment.ID, userIdentifier)
	}

	existingAssignment, err := s.model.GetAssignment(ctx, experiment.Experiment.ID, userIdentifier)
	if err != nil {
		return nil, err
	}

	var selectedVariant *models.ExperimentVariant
	if existingAssignment != nil {
		for _, variant := range experiment.Variants {
			if variant.ID == existingAssignment.VariantID {
				selectedVariant = &variant
				break
			}
		}
	}

	if selectedVariant == nil {
		selectedVariant = selectVariantForUser(experimentKey, userIdentifier, experiment.Variants)
		if selectedVariant == nil {
			return nil, errors.New("no variants available for experiment")
		}

		existingAssignment, err = s.model.CreateAssignment(ctx, experiment.Experiment.ID, selectedVariant.ID, userIdentifier, requestPath)
		if err != nil {
			return nil, err
		}
	}

	return &AssignmentResult{
		Experiment: experiment.Experiment,
		Variant:    *selectedVariant,
		Assignment: *existingAssignment,
	}, nil
}

func (s *ExperimentService) TrackEvent(ctx context.Context, req TrackEventRequest) (*models.ExperimentEvent, *models.ExperimentVariant, error) {
	if strings.TrimSpace(req.EventName) == "" {
		return nil, nil, errors.New("event name is required")
	}

	experiment, err := s.model.GetByKey(ctx, req.ExperimentKey)
	if err != nil {
		return nil, nil, err
	}

	var targetVariant *models.ExperimentVariant
	if req.VariantKey != "" {
		for _, variant := range experiment.Variants {
			if strings.EqualFold(variant.VariantKey, req.VariantKey) {
				targetVariant = &variant
				break
			}
		}
	}

	if targetVariant == nil {
		existingAssignment, err := s.model.GetAssignment(ctx, experiment.Experiment.ID, req.UserID)
		if err != nil {
			return nil, nil, err
		}

		if existingAssignment != nil {
			for _, variant := range experiment.Variants {
				if variant.ID == existingAssignment.VariantID {
					targetVariant = &variant
					break
				}
			}
		}
	}

	if targetVariant == nil {
		assignment, err := s.AssignVariant(ctx, req.ExperimentKey, req.UserID, req.RequestPath, false)
		if err != nil {
			return nil, nil, err
		}
		targetVariant = &assignment.Variant
	}

	event, err := s.model.RecordEvent(ctx, experiment.Experiment.ID, targetVariant.ID, req.UserID, req.EventName, req.Metadata)
	if err != nil {
		return nil, nil, err
	}

	return event, targetVariant, nil
}

func selectVariantForUser(experimentKey, userIdentifier string, variants []models.ExperimentVariant) *models.ExperimentVariant {
	if len(variants) == 0 {
		return nil
	}

	totalWeight := 0
	for _, variant := range variants {
		if variant.Weight > 0 {
			totalWeight += variant.Weight
		}
	}

	if totalWeight == 0 {
		for _, variant := range variants {
			if variant.IsControl {
				return &variant
			}
		}
		return &variants[0]
	}

	seed := hashIdentifier(experimentKey, userIdentifier)
	bucket := int(seed % uint32(totalWeight))
	running := 0

	for _, variant := range variants {
		if variant.Weight <= 0 {
			continue
		}
		running += variant.Weight
		if bucket < running {
			return &variant
		}
	}

	return &variants[len(variants)-1]
}

func hashIdentifier(experimentKey, userIdentifier string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(experimentKey)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(userIdentifier)))
	return h.Sum32()
}

func EnsureIdentifier(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed != "" {
		return trimmed
	}
	return strings.ToLower(time.Now().Format("20060102T150405.000000000"))
}
