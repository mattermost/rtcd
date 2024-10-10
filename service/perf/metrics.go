// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package perf

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	metricsSubSystemRTC       = "rtc"
	metricsSubSystemRTCClient = "rtc_client"
	metricsSubSystemWS        = "ws"
)

var (
	latencyBuckets = []float64{.001, .005, .0075, .01, .025, .05, .075, .1, .25, .3, .4, .5, .75, 1}
	lossBuckets    = []float64{.001, .005, .0075, .01, .025, .05, .075, .1, .25, .5, .75, 1}
)

type Metrics struct {
	registry *prometheus.Registry

	RTPTracks            *prometheus.GaugeVec
	RTPTrackWrites       *prometheus.HistogramVec
	RTCSessions          *prometheus.GaugeVec
	RTCConnStateCounters *prometheus.CounterVec
	RTCErrors            *prometheus.CounterVec

	RTCClientLoss   *prometheus.HistogramVec
	RTCClientRTT    *prometheus.HistogramVec
	RTCClientJitter *prometheus.HistogramVec

	WSConnections     *prometheus.GaugeVec
	WSMessageCounters *prometheus.CounterVec
}

func NewMetrics(namespace string, registry *prometheus.Registry) *Metrics {
	var m Metrics

	if registry != nil {
		m.registry = registry
	} else {
		m.registry = prometheus.NewRegistry()
		m.registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{
			Namespace: namespace,
		}))
		m.registry.MustRegister(collectors.NewGoCollector())
	}

	m.RTPTracks = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: metricsSubSystemRTC,
			Name:      "rtp_tracks_total",
			Help:      "Total number of active RTP tracks",
		},
		[]string{"groupID", "direction", "type"},
	)
	m.registry.MustRegister(m.RTPTracks)

	m.RTPTrackWrites = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: metricsSubSystemRTC,
			Name:      "rtp_tracks_writes_time",
			Help:      "Time taken to write to RTP tracks",
			Buckets:   latencyBuckets,
		},
		[]string{"groupID", "type"},
	)
	m.registry.MustRegister(m.RTPTrackWrites)

	m.RTCSessions = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: metricsSubSystemRTC,
			Name:      "sessions_total",
			Help:      "Total number of active RTC sessions",
		},
		[]string{"groupID"},
	)
	m.registry.MustRegister(m.RTCSessions)

	m.RTCConnStateCounters = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: metricsSubSystemRTC,
			Name:      "conn_states_total",
			Help:      "Total number of RTC connection state changes",
		},
		[]string{"type"},
	)
	m.registry.MustRegister(m.RTCConnStateCounters)

	m.RTCErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: metricsSubSystemRTC,
			Name:      "errors_total",
			Help:      "Total number of RTC related errors",
		},
		[]string{"groupID", "type"},
	)
	m.registry.MustRegister(m.RTCErrors)

	m.WSConnections = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: metricsSubSystemWS,
			Name:      "connections_total",
			Help:      "Total number of active WebSocket sessions",
		},
		[]string{"clientID"},
	)
	m.registry.MustRegister(m.WSConnections)

	m.WSMessageCounters = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: metricsSubSystemWS,
			Name:      "messages_total",
			Help:      "Total number of sent/received WebSocket messages",
		},
		[]string{"clientID", "type", "direction"},
	)
	m.registry.MustRegister(m.WSMessageCounters)

	// Client metrics

	m.RTCClientLoss = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: metricsSubSystemRTCClient,
			Name:      "loss_rate",
			Help:      "Client loss rate",
			Buckets:   lossBuckets,
		},
		[]string{"groupID"},
	)
	m.registry.MustRegister(m.RTCClientLoss)

	m.RTCClientRTT = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: metricsSubSystemRTCClient,
			Name:      "rtt",
			Help:      "Client round trip time",
			Buckets:   latencyBuckets,
		},
		[]string{"groupID"},
	)
	m.registry.MustRegister(m.RTCClientRTT)

	m.RTCClientJitter = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: metricsSubSystemRTCClient,
			Name:      "jitter",
			Help:      "Client latency jitter",
			Buckets:   latencyBuckets,
		},
		[]string{"groupID"},
	)
	m.registry.MustRegister(m.RTCClientJitter)

	return &m
}

func (m *Metrics) IncRTCSessions(groupID string) {
	m.RTCSessions.With(prometheus.Labels{"groupID": groupID}).Inc()
}

func (m *Metrics) DecRTCSessions(groupID string) {
	m.RTCSessions.With(prometheus.Labels{"groupID": groupID}).Dec()
}

func (m *Metrics) IncRTCConnState(state string) {
	m.RTCConnStateCounters.With(prometheus.Labels{"type": state}).Inc()
}

func (m *Metrics) IncRTCErrors(groupID string, errType string) {
	m.RTCErrors.With(prometheus.Labels{"type": errType, "groupID": groupID}).Inc()
}

func (m *Metrics) IncRTPTracks(groupID, direction, trackType string) {
	m.RTPTracks.With(prometheus.Labels{"groupID": groupID, "direction": direction, "type": trackType}).Inc()
}

func (m *Metrics) DecRTPTracks(groupID, direction, trackType string) {
	m.RTPTracks.With(prometheus.Labels{"groupID": groupID, "direction": direction, "type": trackType}).Dec()
}

func (m *Metrics) IncWSConnections(clientID string) {
	m.WSConnections.With(prometheus.Labels{"clientID": clientID}).Inc()
}

func (m *Metrics) DecWSConnections(clientID string) {
	m.WSConnections.With(prometheus.Labels{"clientID": clientID}).Dec()
}

func (m *Metrics) IncWSMessages(clientID, msgType, direction string) {
	m.WSMessageCounters.With(prometheus.Labels{"clientID": clientID, "type": msgType, "direction": direction}).Inc()
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) ObserveRTPTracksWrite(groupID, trackType string, dur float64) {
	m.RTPTrackWrites.With(prometheus.Labels{"groupID": groupID, "type": trackType}).Observe(dur)
}

func (m *Metrics) ObserveRTCClientLossRate(groupID string, val float64) {
	m.RTCClientLoss.With(prometheus.Labels{"groupID": groupID}).Observe(val)
}

func (m *Metrics) ObserveRTCClientRTT(groupID string, val float64) {
	m.RTCClientRTT.With(prometheus.Labels{"groupID": groupID}).Observe(val)
}

func (m *Metrics) ObserveRTCClientJitter(groupID string, val float64) {
	m.RTCClientJitter.With(prometheus.Labels{"groupID": groupID}).Observe(val)
}
