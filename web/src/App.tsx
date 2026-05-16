import { HashRouter, Routes, Route, Navigate } from 'react-router-dom'
import Layout from '@/components/Layout'
import DevicesPage from '@/pages/Devices'
import DiscoveredPage from '@/pages/Discovered'
import PluginsPage from '@/pages/Plugins'
import ProtocolPage from '@/pages/Protocol'
import SettingsPage from '@/pages/Settings'
import { useThemeSync } from '@/lib/uiPrefs'

export default function App() {
  useThemeSync()
  return (
    <HashRouter>
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route index element={<Navigate to="/devices" replace />} />
          <Route path="devices" element={<DevicesPage />} />
          <Route path="discovered" element={<DiscoveredPage />} />
          <Route path="protocol" element={<ProtocolPage />} />
          <Route path="plugins" element={<PluginsPage />} />
          <Route path="settings" element={<SettingsPage />} />
        </Route>
      </Routes>
    </HashRouter>
  )
}
