package hls

// ActiveCount is how many streams are being converted right now, so slow
// background work can give way to them.
func (m *Manager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, s := range m.session {
		s.mu.Lock()
		if s.job != nil && !s.job.finished() {
			n++
		}
		s.mu.Unlock()
	}
	return n
}
