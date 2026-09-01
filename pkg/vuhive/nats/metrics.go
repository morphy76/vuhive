//go:build nats

package nats

import (
	"time"

	"github.com/morphy76/vuhive/pkg/vuhive"
)

func pubTags(subject string, status string) vuhive.Tags {
	return vuhive.Tags{
		"subject": subject,
		"status":  status,
	}
}

func reqTags(subject string, status string) vuhive.Tags {
	return vuhive.Tags{
		"subject": subject,
		"status":  status,
	}
}

func subTags(subject string, status string) vuhive.Tags {
	return vuhive.Tags{
		"subject": subject,
		"status":  status,
	}
}

func recordPubMetrics(collector vuhive.MetricsCollector, prefix string, subject string, duration time.Duration, bytesCount int, err error) {
	if collector == nil {
		return
	}

	status := "ok"
	if err != nil {
		status = "error"
	}
	tags := pubTags(subject, status)

	collector.Duration(prefix+MetricSuffixPubDuration, tags).Observe(duration)
	collector.Counter(prefix+MetricSuffixPubTotal, tags).Inc()

	if bytesCount > 0 {
		collector.Counter(prefix+MetricSuffixPubBytes, vuhive.Tags{"subject": subject}).Add(int64(bytesCount))
	}

	if err != nil {
		collector.Rate(prefix+MetricSuffixPubFailed, tags).Add(1, 1)
	} else {
		collector.Rate(prefix+MetricSuffixPubFailed, tags).Add(0, 1)
	}
}

func recordReqMetrics(collector vuhive.MetricsCollector, prefix string, subject string, duration time.Duration, bytesCount int, err error) {
	if collector == nil {
		return
	}

	status := "ok"
	if err != nil {
		status = "error"
	}
	tags := reqTags(subject, status)

	collector.Duration(prefix+MetricSuffixReqDuration, tags).Observe(duration)
	collector.Counter(prefix+MetricSuffixPubTotal, tags).Inc()

	if bytesCount > 0 {
		collector.Counter(prefix+MetricSuffixPubBytes, vuhive.Tags{"subject": subject}).Add(int64(bytesCount))
	}

	if err != nil {
		collector.Rate(prefix+MetricSuffixPubFailed, tags).Add(1, 1)
	} else {
		collector.Rate(prefix+MetricSuffixPubFailed, tags).Add(0, 1)
	}
}

func recordSubMetrics(collector vuhive.MetricsCollector, prefix string, subject string, bytesCount int, err error) {
	if collector == nil {
		return
	}

	status := "ok"
	if err != nil {
		status = "error"
	}
	tags := subTags(subject, status)

	if err == nil {
		collector.Counter(prefix+MetricSuffixSubReceivedTotal, tags).Inc()
		if bytesCount > 0 {
			collector.Counter(prefix+MetricSuffixSubBytes, vuhive.Tags{"subject": subject}).Add(int64(bytesCount))
		}
		collector.Rate(prefix+MetricSuffixSubFailed, tags).Add(0, 1)
	} else {
		collector.Rate(prefix+MetricSuffixSubFailed, tags).Add(1, 1)
	}
}
