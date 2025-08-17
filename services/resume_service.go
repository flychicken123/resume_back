package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"resumeai/models"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type ResumeService struct {
	resumeHistoryModel *models.ResumeHistoryModel
	s3Service          *S3Service
}

func NewResumeService(resumeHistoryModel *models.ResumeHistoryModel, s3Service *S3Service) *ResumeService {
	return &ResumeService{
		resumeHistoryModel: resumeHistoryModel,
		s3Service:          s3Service,
	}
}

func (s *ResumeService) RecordDownload(userID int, filename, s3Path string) error {
	// Create resume name from filename
	resumeName := strings.TrimSuffix(filename, ".pdf")
	resumeName = strings.ReplaceAll(resumeName, "_", " ")
	resumeName = cases.Title(language.English).String(resumeName)

	// Add to history
	_, err := s.resumeHistoryModel.Create(userID, resumeName, s3Path)
	if err != nil {
		return fmt.Errorf("failed to save to resume history: %v", err)
	}

	// Clean up old resumes (keep only last 3)
	err = s.resumeHistoryModel.CleanupOldResumes(userID, 3)
	if err != nil {
		return fmt.Errorf("failed to cleanup old resumes: %v", err)
	}

	return nil
}

func (s *ResumeService) GeneratePresignedURL(filename string) (string, error) {
	key := "resumes/" + filename
	return s.s3Service.GeneratePresignedURL(key)
}

func (s *ResumeService) UploadPDF(pdfPath, filename string) (string, error) {
	key := "resumes/" + filename
	return s.s3Service.UploadFile(pdfPath, key)
}

func (s *ResumeService) EnsureOutputDirectory() error {
	saveDir := "./static"
	return os.MkdirAll(saveDir, os.ModePerm)
}

func (s *ResumeService) GenerateUniqueFilename(extension string) string {
	return fmt.Sprintf("resume_%d%s", time.Now().UnixNano(), extension)
}

func (s *ResumeService) GetPDFPath(filename string) string {
	return filepath.Join("./static", filename)
}
