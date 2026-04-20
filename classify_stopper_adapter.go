package main

import "resumeai/services"

// classifyStopperAdapter bridges the two classifier services so a single admin
// endpoint can halt both paths.
type classifyStopperAdapter struct {
	backfill  *services.JobClassifyBackfillService
	ingestion *services.JobIngestionService
}

func (a *classifyStopperAdapter) CancelBackfill() bool {
	if a.backfill == nil {
		return false
	}
	return a.backfill.Cancel()
}

func (a *classifyStopperAdapter) PauseIngestionClassifier() {
	if a.ingestion == nil {
		return
	}
	a.ingestion.PauseClassifier()
}

func (a *classifyStopperAdapter) IsIngestionClassifierRunning() bool {
	if a.ingestion == nil {
		return false
	}
	return !a.ingestion.IsClassifierPaused()
}
