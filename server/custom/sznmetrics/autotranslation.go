// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.

package sznmetrics

func (m *SznMetrics) ObserveAutoTranslateTranslateDuration(objectType string, elapsed float64) {
}

func (m *SznMetrics) ObserveAutoTranslateLinguaDetectionDuration(elapsed float64) {
}

func (m *SznMetrics) ObserveAutoTranslateProviderCallDuration(provider, result string, elapsed float64) {
}

func (m *SznMetrics) SetAutoTranslateQueueDepth(depth float64) {
}

func (m *SznMetrics) ObserveAutoTranslateWorkerTaskDuration(elapsed float64) {
}

func (m *SznMetrics) AddAutoTranslateRecoveryStuckFound(count float64) {
}

func (m *SznMetrics) IncrementAutoTranslateNormHash(result string) {
}
