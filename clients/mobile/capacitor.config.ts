import type { CapacitorConfig } from '@capacitor/cli'

// The app ships the web client built from ../../web and talks to whichever
// server the user enters on first launch (stored with @capacitor/preferences).
const config: CapacitorConfig = {
  appId: 'com.wittywilla.wittymedia',
  appName: 'WittyMedia',
  webDir: 'www',
  server: { androidScheme: 'https', cleartext: true },
  ios: { contentInset: 'always', backgroundColor: '#0f1115' },
  android: { backgroundColor: '#0f1115', allowMixedContent: true },
}
export default config
