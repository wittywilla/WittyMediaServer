package modules

// LogCapability is where a module's manifest lists the advanced logging options it
// offers, each as "id|label|description". The module prints the detail when asked
// by WITTYMODULE_LOG_TOPICS. See docs/MODULES.md.
const LogCapability = "logTopics"

// LogTopics is what the version of a module in use says it can log.
func (m *Manager) LogTopics(id string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	if e == nil || e.man == nil {
		return nil
	}
	return e.man.Capabilities[LogCapability]
}
