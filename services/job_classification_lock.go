package services

import "sync"

// jobClassificationRunMu serializes all job-classification producers. Ingest
// classification and admin backfills share the same Gemini quota and write the
// same columns, so only one run should be active at a time.
var jobClassificationRunMu sync.Mutex
