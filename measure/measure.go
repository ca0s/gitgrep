package measure

import "time"

type TimeMeasure struct {
	started  time.Time
	finished time.Time
}

func (m *TimeMeasure) Start() time.Time {
	m.started = time.Now()
	return m.started
}

func (m *TimeMeasure) End() time.Time {
	m.finished = time.Now()
	return m.finished
}

func (m *TimeMeasure) Ellpsed() time.Duration {
	return m.finished.Sub(m.started)
}
