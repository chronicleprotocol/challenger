package core

import "github.com/prometheus/client_golang/prometheus"

const prometheusNamespace = "challenger"

var ErrorsCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
	Namespace: prometheusNamespace,
	Name:      "errors_total",
	Help:      "Challenger Errors Counter",
}, []string{"address", "from", "error"})

var ChallengeCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
	Namespace: prometheusNamespace,
	Name:      "challenges_total",
	Help:      "Number of challenges made",
}, []string{"address", "from", "tx"})

var LastScannedBlockGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: prometheusNamespace,
	Name:      "last_scanned_block",
	Help:      "Last scanned block",
}, []string{"address", "from"})
