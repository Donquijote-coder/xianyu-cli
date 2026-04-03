package utils

import (
	"math"
	"math/rand"
	"net/http"
	"time"
)

// DefaultHeaders returns consistent browser fingerprint headers.
func DefaultHeaders() http.Header {
	h := http.Header{}
	h.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36")
	h.Set("Sec-Ch-Ua", `"Not(A:Brand";v="99", "Google Chrome";v="133", "Chromium";v="133"`)
	h.Set("Sec-Ch-Ua-Mobile", "?0")
	h.Set("Sec-Ch-Ua-Platform", `"macOS"`)
	h.Set("Accept", "application/json")
	h.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	h.Set("Origin", GoofishOrigin)
	h.Set("Referer", GoofishReferer)
	h.Set("Sec-Fetch-Dest", "empty")
	h.Set("Sec-Fetch-Mode", "cors")
	h.Set("Sec-Fetch-Site", "same-site")
	return h
}

// AntiDetect provides anti-detection utilities for API requests.
type AntiDetect struct {
	JitterMean    float64
	JitterStddev  float64
	ReadingChance float64
	ReadingMin    float64
	ReadingMax    float64
}

// NewAntiDetect creates an AntiDetect with default settings.
func NewAntiDetect() *AntiDetect {
	return &AntiDetect{
		JitterMean:    1.2,
		JitterStddev:  0.3,
		ReadingChance: 0.05,
		ReadingMin:    2.0,
		ReadingMax:    5.0,
	}
}

// JitterDelay applies a Gaussian-distributed delay to mimic human timing.
func (a *AntiDetect) JitterDelay() {
	delay := math.Max(0.5, rand.NormFloat64()*a.JitterStddev+a.JitterMean)
	if rand.Float64() < a.ReadingChance {
		delay += a.ReadingMin + rand.Float64()*(a.ReadingMax-a.ReadingMin)
	}
	time.Sleep(time.Duration(delay * float64(time.Second)))
}

// BackoffDelay applies exponential backoff for rate-limit retries.
func (a *AntiDetect) BackoffDelay(attempt int) {
	delay := math.Min(60.0, math.Pow(2, float64(attempt))+rand.Float64())
	time.Sleep(time.Duration(delay * float64(time.Second)))
}

// GaussianJitter returns a jitter duration based on mean and stddev ratio.
func GaussianJitter(mean float64, stddevRatio float64) time.Duration {
	delay := math.Max(0.5, rand.NormFloat64()*mean*stddevRatio+mean)
	return time.Duration(delay * float64(time.Second))
}
