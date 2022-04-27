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
	metricsSubSystemRTC = "rtc"
	metricsSubSystemWS  = "ws"
)

type Metrics struct {
	registry *prometheus.Registry

	RTPPacketCounters      *prometheus.CounterVec
	RTPPacketBytesCounters *prometheus.CounterVec
	RTCSessions            *prometheus.GaugeVec
	RTCConnStateCounters   *prometheus.CounterVec

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

	m.RTPPacketCounters = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: metricsSubSystemRTC,
			Name:      "rtp_packets_total",
			Help:      "Total number of sent/received RTP packets",
		},
		[]string{"direction", "type"},
	)
	m.registry.MustRegister(m.RTPPacketCounters)

	m.RTPPacketBytesCounters = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: metricsSubSystemRTC,
			Name:      "rtp_bytes_total",
			Help:      "Total number of sent/received RTP packet bytes",
		},
		[]string{"direction", "type"},
	)
	m.registry.MustRegister(m.RTPPacketBytesCounters)

	m.RTCSessions = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: metricsSubSystemRTC,
			Name:      "sessions_total",
			Help:      "Total number of active RTC sessions",
		},
		[]string{"groupID", "callID"},
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

	return &m
}

func (m *Metrics) IncRTCSessions(groupID string, callID string) {
	m.RTCSessions.With(prometheus.Labels{"groupID": groupID, "callID": callID}).Inc()
}

func (m *Metrics) DecRTCSessions(groupID string, callID string) {
	m.RTCSessions.With(prometheus.Labels{"groupID": groupID, "callID": callID}).Dec()
}

func (m *Metrics) IncRTCConnState(state string) {
	m.RTCConnStateCounters.With(prometheus.Labels{"type": state}).Inc()
}

func (m *Metrics) IncRTPPackets(direction, trackType string) {
	m.RTPPacketCounters.With(prometheus.Labels{"direction": direction, "type": trackType}).Inc()
}

func (m *Metrics) AddRTPPacketBytes(direction, trackType string, value int) {
	m.RTPPacketBytesCounters.With(prometheus.Labels{"direction": direction, "type": trackType}).Add(float64(value))
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
