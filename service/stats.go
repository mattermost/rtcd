// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	model "github.com/prometheus/client_model/go"
)

func (s *Service) getStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	data := newHTTPData()
	defer s.httpAudit("getStats", data, w, r)

	clientID, code, err := s.authHandler(w, r)
	if err != nil {
		data.err = err.Error()
		data.code = code
		return
	}

	var m = &model.Metric{}

	metric, err := s.metrics.RTCCalls.GetMetricWith(prometheus.Labels{"groupID": clientID})
	if err != nil {
		data.err = err.Error()
		data.code = http.StatusInternalServerError
		return
	}
	if err := metric.Write(m); err != nil {
		data.err = err.Error()
		data.code = http.StatusInternalServerError
		return
	}
	data.resData["calls"] = m.Gauge.GetValue()

	metric, err = s.metrics.RTCSessions.GetMetricWith(prometheus.Labels{"groupID": clientID})
	if err != nil {
		data.err = err.Error()
		data.code = http.StatusInternalServerError
		return
	}
	if err := metric.Write(m); err != nil {
		data.err = err.Error()
		data.code = http.StatusInternalServerError
		return
	}
	data.resData["sessions"] = m.Gauge.GetValue()

	data.code = http.StatusOK
}
