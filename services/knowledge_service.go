package services

import (
	"context"
	"fmt"
	"time"

	"resumeai/models"
	"resumeai/utils"
)

type KnowledgeService struct {
	model    *models.KnowledgeBaseModel
	embedSvc *EmbeddingService
	logger   *utils.Logger
}

func NewKnowledgeService(model *models.KnowledgeBaseModel, embedSvc *EmbeddingService, logger *utils.Logger) *KnowledgeService {
	return &KnowledgeService{model: model, embedSvc: embedSvc, logger: logger}
}

func (s *KnowledgeService) AddEntry(ctx context.Context, title, category, content string) (int, error) {
	id, err := s.model.Insert(title, category, content)
	if err != nil {
		return 0, fmt.Errorf("insert failed: %w", err)
	}
	if s.embedSvc != nil {
		emb, err := s.embedSvc.EmbedDocument(ctx, title+" "+content)
		if err != nil {
			s.logger.Warn("failed to embed knowledge entry", map[string]interface{}{"id": id, "error": err.Error()})
		} else {
			_ = s.model.UpdateEmbedding(id, emb)
		}
	}
	return id, nil
}

func (s *KnowledgeService) UpdateEntry(ctx context.Context, id int, title, category, content string) error {
	if err := s.model.Update(id, title, category, content); err != nil {
		return err
	}
	if s.embedSvc != nil {
		emb, err := s.embedSvc.EmbedDocument(ctx, title+" "+content)
		if err != nil {
			s.logger.Warn("failed to re-embed knowledge entry", map[string]interface{}{"id": id, "error": err.Error()})
		} else {
			_ = s.model.UpdateEmbedding(id, emb)
		}
	}
	return nil
}

func (s *KnowledgeService) Delete(id int) error {
	return s.model.Delete(id)
}

func (s *KnowledgeService) List() ([]models.KnowledgeEntry, error) {
	return s.model.ListAll()
}

func (s *KnowledgeService) Search(ctx context.Context, query string, limit int) ([]models.KnowledgeEntry, error) {
	if s.embedSvc == nil {
		return nil, fmt.Errorf("embedding service not configured")
	}
	emb, err := s.embedSvc.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query embedding failed: %w", err)
	}
	return s.model.SearchByVector(emb, limit, 0.5)
}

func (s *KnowledgeService) BackfillEmbeddings(ctx context.Context) (int, error) {
	entries, err := s.model.ListWithNullEmbedding()
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, e := range entries {
		if s.embedSvc == nil {
			break
		}
		emb, err := s.embedSvc.EmbedDocument(ctx, e.Title+" "+e.Content)
		if err != nil {
			s.logger.Warn("backfill embed failed", map[string]interface{}{"id": e.ID, "error": err.Error()})
			continue
		}
		if err := s.model.UpdateEmbedding(e.ID, emb); err != nil {
			continue
		}
		processed++
		time.Sleep(100 * time.Millisecond) // rate limit
	}
	return processed, nil
}

func (s *KnowledgeService) SeedIfEmpty(ctx context.Context) {
	count, err := s.model.Count()
	if err != nil || count > 0 {
		return
	}
	s.logger.Info("seeding knowledge base", map[string]interface{}{"entries": len(knowledgeSeedData)})
	for _, entry := range knowledgeSeedData {
		_, err := s.AddEntry(ctx, entry.Title, entry.Category, entry.Content)
		if err != nil {
			s.logger.Warn("seed entry failed", map[string]interface{}{"title": entry.Title, "error": err.Error()})
		}
		time.Sleep(100 * time.Millisecond) // rate limit embedding calls
	}
	s.logger.Info("knowledge base seeded", map[string]interface{}{"entries": len(knowledgeSeedData)})
}
